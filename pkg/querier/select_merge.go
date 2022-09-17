package querier

import (
	"container/heap"
	"context"

	"github.com/grafana/dskit/multierror"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/ingester/clientpool"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
)

var _ = iter.Iterator[ProfileWithLabels]((*stacktraceMergeIterator)(nil))

type ProfileWithLabels struct {
	Timestamp int64
	firemodel.Labels
	IngesterAddr string
}

// StacktraceMergeIterator is an iterator of profiles for merging stacktraces.
// While iterating through Profiles you can use Keep() to mark the current profile to include
// in the final stacktraces merge result.
// You can also use At() to get the current profile.
// Result should only be called after the iterator is exhausted.
type StacktraceMergeIterator interface {
	iter.Iterator[ProfileWithLabels]
	Keep()
	Result() (*ingestv1.MergeProfilesStacktracesResult, error)
}

type stacktraceMergeIterator struct {
	ctx          context.Context
	bidi         clientpool.BidiClientMergeProfilesStacktraces
	ingesterAddr string

	err      error
	curr     *ingestv1.ProfileSets
	currIdx  int
	keep     []bool
	keepSent bool // keepSent is true if we have sent the keep request to the ingester.

	result *ingestv1.MergeProfilesStacktracesResult
}

// NewStacktraceMergeIterator return a new iterator that merges stacktraces of profile.
// Merging or querying stacktraces is expensive, we only merge the stacktraces of the profiles that are kept.
func NewStacktraceMergeIterator(ctx context.Context, r responseFromIngesters[clientpool.BidiClientMergeProfilesStacktraces]) StacktraceMergeIterator {
	return &stacktraceMergeIterator{
		bidi:         r.response,
		ingesterAddr: r.addr,
		keepSent:     true, // at the start we don't send a keep request.
		ctx:          ctx,
	}
}

func (s *stacktraceMergeIterator) Next() bool {
	if s.curr == nil || s.currIdx >= len(s.curr.Profiles)-1 {
		// ensure we send keep before reading next batch.
		// the iterator only need to precise profile to keep, not the ones to drop.
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
		// reset selections to none
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

func (s *stacktraceMergeIterator) Keep() {
	s.keep[s.currIdx] = true
}

func (s *stacktraceMergeIterator) At() ProfileWithLabels {
	return ProfileWithLabels{
		Timestamp:    s.curr.Profiles[s.currIdx].Timestamp,
		Labels:       s.curr.LabelsSets[s.curr.Profiles[s.currIdx].LabelIndex].Labels,
		IngesterAddr: s.ingesterAddr,
	}
}

func (s *stacktraceMergeIterator) Result() (*ingestv1.MergeProfilesStacktracesResult, error) {
	resp, err := s.bidi.Receive()
	if err != nil {
		return nil, err
	}
	if err := s.bidi.CloseResponse(); err != nil {
		s.err = err
	}
	return resp.Result, nil
}

func (s *stacktraceMergeIterator) Err() error {
	return s.err
}

func (s *stacktraceMergeIterator) Close() error {
	// Only close the Send side since we need to get the final result.
	var errs multierror.MultiError
	if err := s.bidi.CloseRequest(); err != nil {
		errs = append(errs, err)
	}
	return errs.Err()
}

// ProfileIteratorHeap is a heap that sorts profiles by timestamp then labels at the top.
type ProfileIteratorHeap []StacktraceMergeIterator

func (h ProfileIteratorHeap) Len() int { return len(h) }
func (h ProfileIteratorHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
func (h ProfileIteratorHeap) Peek() StacktraceMergeIterator { return h[0] }
func (h *ProfileIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(StacktraceMergeIterator))
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

func newProfilesHeap(its []StacktraceMergeIterator) *ProfileIteratorHeap {
	heap := make(ProfileIteratorHeap, 0, len(its))
	return &heap
}

// skipDuplicates iterates through the iterator and skip duplicates.
func skipDuplicates(its []StacktraceMergeIterator) error {
	profilesHeap := newProfilesHeap(its)
	tuples := make([]StacktraceMergeIterator, 0, len(its))

	if err := requeueAsync(profilesHeap, its...); err != nil {
		return err
	}

	for {
		if profilesHeap.Len() == 0 {
			return nil
		}
		if profilesHeap.Len() == 1 {
			profilesHeap.Peek().Keep()
			if !profilesHeap.Peek().Next() {
				profilesHeap.Pop()
			}
			continue
		}

		for profilesHeap.Len() > 0 {
			next := profilesHeap.Peek()
			value := next.At()
			if len(tuples) > 0 && (tuples[0].At().Timestamp != value.Timestamp || firemodel.CompareLabelPairs(tuples[0].At().Labels, value.Labels) != 0) {
				break
			}
			heap.Pop(profilesHeap)
			tuples = append(tuples, next)
		}

		// right now we pick the first ingester.
		tuples[0].Keep()
		if err := requeueAsync(profilesHeap, tuples...); err != nil {
			return err
		}
		tuples = tuples[:0]
	}
}

// requeueAsync multiple iterators, in parallel, it will only requeue iterators that are not done.
// It will close the iterators that are done.
func requeueAsync(h heap.Interface, eis ...StacktraceMergeIterator) error {
	sync := lo.Synchronize()
	errs := make([]chan error, len(eis))
	for i, ei := range eis {
		ei := ei
		errs[i] = lo.Async(func() error {
			if ei.Next() {
				sync.Do(func() {
					heap.Push(h, ei)
				})
				return nil
			}
			ei.Close()
			return ei.Err()
		})
	}
	var multiErr multierror.MultiError
	// wait for all async.
	for _, err := range errs {
		multiErr.Add(<-err)
	}
	return multiErr.Err()
}

// selectMergeStacktraces selects the  profile from each ingester by deduping them and request merges of stacktraces of them.
func selectMergeStacktraces(ctx context.Context, responses []responseFromIngesters[clientpool.BidiClientMergeProfilesStacktraces]) ([]stacktraces, error) {
	iters := make([]StacktraceMergeIterator, 0, len(responses))
	for _, resp := range responses {
		iters = append(iters, NewStacktraceMergeIterator(ctx, resp))
	}

	if err := skipDuplicates(iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([]*ingestv1.MergeProfilesStacktracesResult, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range iters {
		iter := iter
		g.Go(func() error {
			result, err := iter.Result()
			if err != nil {
				return err
			}
			s.Do(func() {
				results = append(results, result)
			})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return mergeProfilesStacktracesResult(results), nil
}

// mergeProfilesStacktracesResult merges the results of multiple MergeProfilesStacktraces into a single result.
func mergeProfilesStacktracesResult(results []*ingestv1.MergeProfilesStacktracesResult) []stacktraces {
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
