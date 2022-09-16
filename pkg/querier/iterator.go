package querier

// This file implements iterator.Interface specifics to querier code.
// If you want to use for other types, we should move those to generics.

import (
	"container/heap"
	"context"
	"sync"

	"github.com/grafana/dskit/multierror"
	"golang.org/x/sync/errgroup"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
)

var _ = iter.Iterator[ProfileWithLabels]((*StreamProfileIterator)(nil))

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
	Result() (*ingestv1.MergeProfilesStacktracesResult, error)
	iter.Iterator[ProfileWithLabels]
}

func dedupe(ctx context.Context, responses []responseFromIngesters[BidiClientMergeProfilesStacktraces]) ([]stacktraces, error) {
	iters := make([]SkipIterator, 0, len(responses))
	for _, resp := range responses {
		iters = append(iters, NewStreamProfileIterator(ctx, resp))
	}
	it := NewDedupeProfileIterator(iters)
	defer it.Close()
	for it.Next() {
	}
	if it.Err() != nil {
		return nil, it.Err()
	}

	results := make([]*ingestv1.MergeProfilesStacktracesResult, 0, len(iters))
	var lock sync.Mutex
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range iters {
		iter := iter
		g.Go(func() error {
			result, err := iter.Result()
			if err != nil {
				return err
			}
			lock.Lock()
			results = append(results, result)
			lock.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
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
	keepSent     bool
	ctx          context.Context
}

func NewStreamProfileIterator(ctx context.Context, r responseFromIngesters[BidiClientMergeProfilesStacktraces]) SkipIterator {
	return &StreamProfileIterator{
		bidi:         r.response,
		ingesterAddr: r.addr,
		keepSent:     true,
		ctx:          ctx,
	}
}

func (s *StreamProfileIterator) Next() bool {
	if s.curr == nil || s.currIdx >= len(s.curr.Profiles)-1 {
		// ensure we send keep before reading next batch.
		if !s.keepSent {
			err := s.bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Profiles: s.keep,
			})
			if err != nil {
				s.err = err
				return false
			}
		}
		resp, err := s.bidi.Receive()
		if err != nil {
			s.err = err
			return false
		}

		if resp.SelectedProfiles == nil || len(resp.SelectedProfiles.Profiles) == 0 {
			return false
		}
		s.curr = resp.SelectedProfiles
		if len(s.curr.Profiles) > cap(s.keep) {
			s.keep = make([]bool, len(s.curr.Profiles))
		}
		s.keep = s.keep[:len(s.curr.Profiles)]
		for i := range s.keep {
			s.keep[i] = false
		}
		s.currIdx = 0
		s.keepSent = false
		return true
	}
	s.currIdx++
	return true
}

func (s *StreamProfileIterator) Skip() {
	s.keep[s.currIdx] = false
}

func (s *StreamProfileIterator) Keep() {
	s.keep[s.currIdx] = true
}

func (s *StreamProfileIterator) At() ProfileWithLabels {
	return ProfileWithLabels{
		Timestamp:    s.curr.Profiles[s.currIdx].Timestamp,
		Labels:       s.curr.LabelsSets[s.curr.Profiles[s.currIdx].LabelIndex].Labels,
		IngesterAddr: s.ingesterAddr,
	}
}

func (s *StreamProfileIterator) Result() (*ingestv1.MergeProfilesStacktracesResult, error) {
	resp, err := s.bidi.Receive()
	if err != nil {
		return nil, err
	}
	if err := s.bidi.CloseReceive(); err != nil {
		s.err = err
	}
	return resp.Result, nil
}

func (s *StreamProfileIterator) Err() error {
	return s.err
}

func (s *StreamProfileIterator) Close() error {
	// Only close the Send side since we need to get the final result.
	var errs multierror.MultiError
	if err := s.bidi.CloseSend(); err != nil {
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

	tuples []SkipIterator
}

// NewDedupeProfileIterator creates a new an iterator of ProfileWithLabels while
// iterating it removes duplicate Profile by ID across the set of iterators, but not within.
func NewDedupeProfileIterator(its []SkipIterator) *DedupeProfileIterator {
	heap := make(ProfileIteratorHeap, 0, len(its))
	res := &DedupeProfileIterator{
		heap:   &heap,
		tuples: make([]SkipIterator, 0, len(its)),
	}
	res.requeue(its...)
	return res
}

func (i *DedupeProfileIterator) requeue(eis ...SkipIterator) {
	g, _ := errgroup.WithContext(context.Background())
	topush := make(chan SkipIterator)

	for _, ei := range eis {
		ei := ei
		g.Go(func() error {
			if ei.Next() {
				topush <- ei
				return nil
			}
			ei.Close()
			return ei.Err()
		})
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ei := range topush {
			heap.Push(i.heap, ei)
		}
	}()

	i.errs = append(i.errs, g.Wait())
	close(topush)
	wg.Wait()
}

func (i *DedupeProfileIterator) Next() bool {
	if i.heap.Len() == 0 {
		return false
	}
	if i.heap.Len() == 1 {
		i.heap.Peek().Keep()
		if !i.heap.Peek().Next() {
			i.heap.Pop()
		}
		return true
	}

	for i.heap.Len() > 0 {
		next := i.heap.Peek()
		value := next.At()
		if len(i.tuples) > 0 && (i.tuples[0].At().Timestamp != value.Timestamp || firemodel.CompareLabelPairs(i.tuples[0].At().Labels, value.Labels) != 0) {
			break
		}
		heap.Pop(i.heap)
		i.tuples = append(i.tuples, next)
	}
	// shortcut if we have a single tuple.
	if len(i.tuples) == 1 {
		i.tuples[0].Keep()
		i.requeue(i.tuples[0])
		i.tuples = i.tuples[:0]
		return true
	}

	// todo: we might want to pick based on ingester addr.
	i.tuples[0].Keep()
	// skip the rest.
	for _, t := range i.tuples[1:] {
		t.Skip()
	}
	i.requeue(i.tuples...)
	i.tuples = i.tuples[:0]

	return true
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
