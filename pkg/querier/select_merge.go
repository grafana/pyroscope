package querier

import (
	"container/heap"
	"context"

	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/multierror"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/ingester/clientpool"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/pprof"
)

type ProfileWithLabels struct {
	Timestamp int64
	phlaremodel.Labels
	IngesterAddr string
}

type BidiClientMerge[Req any, Res any] interface {
	Send(Req) error
	Receive() (Res, error)
	CloseRequest() error
	CloseResponse() error
}

type Request interface {
	*ingestv1.MergeProfilesStacktracesRequest | *ingestv1.MergeProfilesLabelsRequest | *ingestv1.MergeProfilesPprofRequest
}

type Response interface {
	*ingestv1.MergeProfilesStacktracesResponse | *ingestv1.MergeProfilesLabelsResponse | *ingestv1.MergeProfilesPprofResponse
}

type MergeResult[R any] interface {
	Result() (R, error)
}
type MergeIterator interface {
	iter.Iterator[ProfileWithLabels]
	Keep()
}

type mergeIterator[R any, Req Request, Res Response] struct {
	ctx          context.Context
	bidi         BidiClientMerge[Req, Res]
	ingesterAddr string

	err      error
	curr     *ingestv1.ProfileSets
	currIdx  int
	keep     []bool
	keepSent bool // keepSent is true if we have sent the keep request to the ingester.
}

// NewMergeIterator return a new iterator that stream profiles and allows to filter them using `Keep` to keep
// only a subset of the profiles for an aggregation result.
// Merging or querying profiles sample values is expensive, we only merge the sample of the profiles that are kept.
func NewMergeIterator[
	R any,
	Req Request,
	Res Response,
](ctx context.Context, r responseFromIngesters[BidiClientMerge[Req, Res]],
) *mergeIterator[R, Req, Res] {
	return &mergeIterator[R, Req, Res]{
		bidi:         r.response,
		ingesterAddr: r.addr,
		keepSent:     true, // at the start we don't send a keep request.
		ctx:          ctx,
	}
}

func (s *mergeIterator[R, Req, Res]) Next() bool {
	if s.curr == nil || s.currIdx >= len(s.curr.Profiles)-1 {
		// ensure we send keep before reading next batch.
		// the iterator only need to precise profile to keep, not the ones to drop.
		if !s.keepSent {
			var err error
			switch bidi := (s.bidi).(type) {
			case BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]:
				err = bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
					Profiles: s.keep,
				})
			case BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]:
				err = bidi.Send(&ingestv1.MergeProfilesLabelsRequest{
					Profiles: s.keep,
				})
			case BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]:
				err = bidi.Send(&ingestv1.MergeProfilesPprofRequest{
					Profiles: s.keep,
				})
			}
			if err != nil {
				s.err = err
				return false
			}
		}
		var selectedProfiles *ingestv1.ProfileSets
		switch bidi := (s.bidi).(type) {
		case BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]:
			res, err := bidi.Receive()
			if err != nil {
				s.err = err
				return false
			}
			selectedProfiles = res.SelectedProfiles
		case BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]:
			res, err := bidi.Receive()
			if err != nil {
				s.err = err
				return false
			}
			selectedProfiles = res.SelectedProfiles
		case BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]:
			res, err := bidi.Receive()
			if err != nil {
				s.err = err
				return false
			}
			selectedProfiles = res.SelectedProfiles
		}

		if selectedProfiles == nil || len(selectedProfiles.Profiles) == 0 {
			return false
		}
		s.curr = selectedProfiles
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

func (s *mergeIterator[R, Req, Res]) Keep() {
	s.keep[s.currIdx] = true
}

func (s *mergeIterator[R, Req, Res]) At() ProfileWithLabels {
	return ProfileWithLabels{
		Timestamp:    s.curr.Profiles[s.currIdx].Timestamp,
		Labels:       s.curr.LabelsSets[s.curr.Profiles[s.currIdx].LabelIndex].Labels,
		IngesterAddr: s.ingesterAddr,
	}
}

func (s *mergeIterator[R, Req, Res]) Result() (R, error) {
	var result R
	switch bidi := (s.bidi).(type) {
	case BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]:
		res, err := bidi.Receive()
		if err != nil {
			s.err = err
			return result, err
		}
		result = any(res.Result).(R)
	case BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]:
		res, err := bidi.Receive()
		if err != nil {
			s.err = err
			return result, err
		}
		result = any(res.Series).(R)
	case BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]:
		res, err := bidi.Receive()
		if err != nil {
			s.err = err
			return result, err
		}
		result = any(res.Result).(R)
	}
	if err := s.bidi.CloseResponse(); err != nil {
		s.err = err
	}
	return result, nil
}

func (s *mergeIterator[R, Req, Res]) Err() error {
	return s.err
}

func (s *mergeIterator[R, Req, Res]) Close() error {
	// Only close the Send side since we need to get the final result.
	var errs multierror.MultiError
	if err := s.bidi.CloseRequest(); err != nil {
		errs = append(errs, err)
	}
	return errs.Err()
}

// ProfileIteratorHeap is a heap that sorts profiles by timestamp then labels at the top.
type ProfileIteratorHeap []MergeIterator

func (h ProfileIteratorHeap) Len() int { return len(h) }
func (h ProfileIteratorHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
func (h ProfileIteratorHeap) Peek() MergeIterator { return h[0] }
func (h *ProfileIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(MergeIterator))
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
		return phlaremodel.CompareLabelPairs(p1.Labels, p2.Labels) < 0
	}
	return p1.Timestamp < p2.Timestamp
}

func newProfilesHeap(its []MergeIterator) *ProfileIteratorHeap {
	heap := make(ProfileIteratorHeap, 0, len(its))
	return &heap
}

// skipDuplicates iterates through the iterator and skip duplicates.
func skipDuplicates(its []MergeIterator) error {
	profilesHeap := newProfilesHeap(its)
	tuples := make([]MergeIterator, 0, len(its))

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
			if len(tuples) > 0 && (tuples[0].At().Timestamp != value.Timestamp || phlaremodel.CompareLabelPairs(tuples[0].At().Labels, value.Labels) != 0) {
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
func requeueAsync(h heap.Interface, eis ...MergeIterator) error {
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

type stacktraces struct {
	locations []string
	value     int64
}

// selectMergeStacktraces selects the  profile from each ingester by deduping them and request merges of stacktraces of them.
func selectMergeStacktraces(ctx context.Context, responses []responseFromIngesters[clientpool.BidiClientMergeProfilesStacktraces]) ([]stacktraces, error) {
	mergeResults := make([]MergeResult[*ingestv1.MergeProfilesStacktracesResult], len(responses))
	iters := make([]MergeIterator, len(responses))
	for i, resp := range responses {
		it := NewMergeIterator[*ingestv1.MergeProfilesStacktracesResult](
			ctx, responseFromIngesters[BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]]{
				addr:     resp.addr,
				response: resp.response,
			})
		iters[i] = it
		mergeResults[i] = it
	}

	if err := skipDuplicates(iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([]*ingestv1.MergeProfilesStacktracesResult, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
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

// selectMergePprofProfile selects the  profile from each ingester by deduping them and request merges of stacktraces in the pprof format.
func selectMergePprofProfile(ctx context.Context, ty *typesv1.ProfileType, responses []responseFromIngesters[clientpool.BidiClientMergeProfilesPprof]) (*googlev1.Profile, error) {
	mergeResults := make([]MergeResult[[]byte], len(responses))
	iters := make([]MergeIterator, len(responses))
	for i, resp := range responses {
		it := NewMergeIterator[[]byte](
			ctx, responseFromIngesters[BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]]{
				addr:     resp.addr,
				response: resp.response,
			})
		iters[i] = it
		mergeResults[i] = it
	}

	if err := skipDuplicates(iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([]*profile.Profile, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
		iter := iter
		g.Go(func() error {
			result, err := iter.Result()
			if err != nil {
				return err
			}
			p, err := profile.ParseUncompressed(result)
			if err != nil {
				return err
			}
			s.Do(func() {
				results = append(results, p)
			})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		empty := &profile.Profile{}
		phlaremodel.SetProfileMetadata(empty, ty)
		return pprof.FromProfile(empty)
	}
	p, err := profile.Merge(results)
	if err != nil {
		return nil, err
	}
	return pprof.FromProfile(p)
}

// mergeProfilesStacktracesResult merges the results of multiple MergeProfilesStacktraces into a single result.
func mergeProfilesStacktracesResult(results []*ingestv1.MergeProfilesStacktracesResult) []stacktraces {
	merge := phlaremodel.MergeBatchMergeStacktraces(results...)
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

type ProfileValue struct {
	Ts         int64
	Lbs        []*typesv1.LabelPair
	LabelsHash uint64
	Value      float64
}

func (p ProfileValue) Labels() phlaremodel.Labels {
	return p.Lbs
}

func (p ProfileValue) Timestamp() model.Time {
	return model.Time(p.Ts)
}

// selectMergeSeries selects the  profile from each ingester by deduping them and request merges of total values.
func selectMergeSeries(ctx context.Context, responses []responseFromIngesters[clientpool.BidiClientMergeProfilesLabels]) (iter.Iterator[ProfileValue], error) {
	mergeResults := make([]MergeResult[[]*typesv1.Series], len(responses))
	iters := make([]MergeIterator, len(responses))
	for i, resp := range responses {
		it := NewMergeIterator[[]*typesv1.Series](
			ctx, responseFromIngesters[BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]]{
				addr:     resp.addr,
				response: resp.response,
			})
		iters[i] = it
		mergeResults[i] = it
	}

	if err := skipDuplicates(iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([][]*typesv1.Series, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
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
	series := phlaremodel.MergeSeries(results...)
	seriesIters := make([]iter.Iterator[ProfileValue], 0, len(series))
	for _, s := range series {
		s := s
		seriesIters = append(seriesIters, newSeriesIterator(s.Labels, s.Points))
	}
	return iter.NewSortProfileIterator(seriesIters), nil
}

type seriesIterator struct {
	point []*typesv1.Point

	curr ProfileValue
}

func newSeriesIterator(lbs []*typesv1.LabelPair, points []*typesv1.Point) *seriesIterator {
	return &seriesIterator{
		point: points,

		curr: ProfileValue{
			Lbs:        lbs,
			LabelsHash: phlaremodel.Labels(lbs).Hash(),
		},
	}
}

func (s *seriesIterator) Next() bool {
	if len(s.point) == 0 {
		return false
	}
	p := s.point[0]
	s.point = s.point[1:]
	s.curr.Ts = p.Timestamp
	s.curr.Value = p.Value
	return true
}

func (s *seriesIterator) At() ProfileValue {
	return s.curr
}

func (s *seriesIterator) Err() error {
	return nil
}

func (s *seriesIterator) Close() error {
	return nil
}
