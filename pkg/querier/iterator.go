package querier

// This file implements iterator.Interface specifics to querier code.
// If you want to use for other types, we should move those to generics.

import (
	"container/heap"

	"github.com/grafana/dskit/multierror"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
)

var (
	_ = iter.Iterator[ProfileWithLabels]((*StreamProfileIterator)(nil))
	_ = iter.Iterator[ProfileWithLabels]((*DedupeProfileIterator)(nil))
)

type BidiClientMergeProfilesStacktraces interface {
	Send(*ingestv1.MergeProfilesStacktracesRequest) error
	Receive() (*ingestv1.MergeProfilesStacktracesResponse, error)
	CloseSend() error
	CloseReceive() error
}

type ProfileWithLabels struct {
	Timestamp int64
	firemodel.Labels
	IngesterAddr string
}

type SkipIterator interface {
	Skip()
	Keep()
	Result() *ingestv1.MergeProfilesStacktracesResult
	iter.Iterator[ProfileWithLabels]
}

func dedupe(responses []responseFromIngesters[BidiClientMergeProfilesStacktraces]) ([]stacktraces, error) {
	iters := make([]SkipIterator, 0, len(responses))
	for _, resp := range responses {
		iters = append(iters, NewStreamProfileIterator(resp))
	}
	it := NewDedupeProfileIterator(iters)
	defer it.Close()
	for it.Next() {
	}
	if it.Err() != nil {
		return nil, it.Err()
	}

	results := make([]*ingestv1.MergeProfilesStacktracesResult, 0, len(iters))
	for _, iter := range iters {
		results = append(results, iter.Result())
	}
	return mergeResult(results), nil
}

func mergeResult(results []*ingestv1.MergeProfilesStacktracesResult) []stacktraces {
	merge := firemodel.MergeBatchMergeStacktraces(results...)
	result := make([]stacktraces, 0, len(merge.Stacktraces))
	for _, st := range merge.Stacktraces {
		locs := make([]string, len(st.FunctionIds))
		for i, id := range st.FunctionIds {
			locs[i] = merge.FunctionNames[id]
		}
		result = append(result, stacktraces{
			value:     st.Value,
			locations: locs,
		})
	}
	return result
}

type StreamProfileIterator struct {
	bidi         BidiClientMergeProfilesStacktraces
	ingesterAddr string
	err          error
	curr         *ingestv1.ProfileSets
	currIdx      int
	keep         []bool
	result       *ingestv1.MergeProfilesStacktracesResult
}

func NewStreamProfileIterator(r responseFromIngesters[BidiClientMergeProfilesStacktraces]) SkipIterator {
	return &StreamProfileIterator{
		bidi:         r.response,
		ingesterAddr: r.addr,
	}
}

func (s *StreamProfileIterator) Next() bool {
	if s.curr == nil || s.currIdx >= len(s.curr.Profiles)-1 {

		resp, err := s.bidi.Receive()
		if err != nil {
			s.err = err
			return false
		}
		if resp.Result != nil {
			s.result = resp.Result
		}
		if resp.SelectedProfiles == nil || len(resp.SelectedProfiles.Profiles) == 0 {
			return false
		}
		s.curr = resp.SelectedProfiles
		if len(s.curr.Profiles) != cap(s.keep) {
			s.keep = make([]bool, len(s.curr.Profiles))
		}
		for i := range s.keep {
			s.keep[i] = false
		}
		s.currIdx = 0
		return true
	}
	s.currIdx++
	return true
}

func (s *StreamProfileIterator) Skip() {
	s.keep[s.currIdx] = false
	if s.currIdx == len(s.curr.Profiles)-1 {
		err := s.bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Profiles: s.keep,
		})
		if err != nil {
			s.err = err
		}
	}
}

func (s *StreamProfileIterator) Keep() {
	s.keep[s.currIdx] = true
	if s.currIdx == len(s.curr.Profiles)-1 {
		err := s.bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Profiles: s.keep,
		})
		if err != nil {
			s.err = err
		}
	}
}

func (s *StreamProfileIterator) At() ProfileWithLabels {
	return ProfileWithLabels{
		Timestamp:    s.curr.Profiles[s.currIdx].Timestamp,
		Labels:       s.curr.LabelsSets[s.curr.Profiles[s.currIdx].LabelIndex].Labels,
		IngesterAddr: s.ingesterAddr,
	}
}

func (s *StreamProfileIterator) Result() *ingestv1.MergeProfilesStacktracesResult {
	return s.result
}

func (s *StreamProfileIterator) Err() error {
	return s.err
}

func (s *StreamProfileIterator) Close() error {
	var errs multierror.MultiError
	if err := s.bidi.CloseSend(); err != nil {
		errs = append(errs, err)
	}
	if err := s.bidi.CloseReceive(); err != nil {
		errs = append(errs, err)
	}
	return errs.Err()
}

type ProfileIteratorHeap []SkipIterator

func (h ProfileIteratorHeap) Len() int { return len(h) }
func (h ProfileIteratorHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
func (h ProfileIteratorHeap) Peek() SkipIterator { return h[0] }
func (h *ProfileIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(SkipIterator))
}

func (h *ProfileIteratorHeap) Pop() interface{} {
	n := len(*h)
	x := (*h)[n-1]
	*h = (*h)[0 : n-1]
	return x
}

func (h ProfileIteratorHeap) Less(i, j int) bool {
	p1, p2 := h[i].At(), h[j].At()
	if p1.Timestamp == p2.Timestamp {
		return firemodel.CompareLabelPairs(p1.Labels, p2.Labels) < 0
	}
	return p1.Timestamp < p2.Timestamp
}

type DedupeProfileIterator struct {
	heap *ProfileIteratorHeap
	errs []error
	curr ProfileWithLabels

	tuples []tuple
}

type tuple struct {
	ProfileWithLabels
	SkipIterator
}

// NewDedupeProfileIterator creates a new an iterator of ProfileWithLabels while
// iterating it removes duplicate Profile by ID across the set of iterators, but not within.
func NewDedupeProfileIterator(its []SkipIterator) iter.Iterator[ProfileWithLabels] {
	heap := make(ProfileIteratorHeap, 0, len(its))
	res := &DedupeProfileIterator{
		heap:   &heap,
		tuples: make([]tuple, 0, len(its)),
	}
	for _, iter := range its {
		res.requeue(iter, false)
	}
	return res
}

func (i *DedupeProfileIterator) requeue(ei iter.Iterator[ProfileWithLabels], advanced bool) {
	if advanced || ei.Next() {
		heap.Push(i.heap, ei)
		return
	}
	ei.Close()
	if err := ei.Err(); err != nil {
		i.errs = append(i.errs, err)
	}
}

func (i *DedupeProfileIterator) Next() bool {
	if i.heap.Len() == 0 {
		return false
	}
	if i.heap.Len() == 1 {
		i.curr = i.heap.Peek().At()
		i.heap.Peek().Keep()
		if !i.heap.Peek().Next() {
			i.heap.Pop()
		}
		return true
	}

	for i.heap.Len() > 0 {
		next := i.heap.Peek()
		value := next.At()
		if len(i.tuples) > 0 && (i.tuples[0].Timestamp != value.Timestamp || firemodel.CompareLabelPairs(i.tuples[0].Labels, value.Labels) != 0) {
			break
		}
		heap.Pop(i.heap)
		i.tuples = append(i.tuples, tuple{
			ProfileWithLabels: value,
			SkipIterator:      next,
		})
	}
	// shortcut if we have a single tuple.
	if len(i.tuples) == 1 {
		i.curr = i.tuples[0].ProfileWithLabels
		i.tuples[0].SkipIterator.Keep()
		i.requeue(i.tuples[0].SkipIterator, false)
		i.tuples = i.tuples[:0]
		return true
	}

	// todo: we might want to pick based on ingester addr.
	t := i.tuples[0]
	t.SkipIterator.Keep()
	i.requeue(t.SkipIterator, false)
	i.curr = t.ProfileWithLabels

	// skip the rest.
	for _, t := range i.tuples[1:] {
		t.SkipIterator.Skip()
		i.requeue(t.SkipIterator, false)
	}
	i.tuples = i.tuples[:0]

	return true
}

func (i *DedupeProfileIterator) At() ProfileWithLabels {
	return i.curr
}

func (i *DedupeProfileIterator) Err() error {
	return multierror.New(i.errs...).Err()
}

func (i *DedupeProfileIterator) Close() error {
	var errs []error
	for _, s := range *i.heap {
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return multierror.New(errs...).Err()
}
