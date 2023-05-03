package querier

import (
	"context"
	"math"
	"sync"

	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	otlog "github.com/opentracing/opentracing-go/log"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/ingester/clientpool"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/pprof"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/loser"
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
	iter.Iterator[*ProfileWithLabels]
	Keep()
}

type keepResponse struct {
	*ingestv1.MergeProfilesStacktracesRequest
	*ingestv1.MergeProfilesLabelsRequest
	*ingestv1.MergeProfilesPprofRequest
}
type mergeIterator[R any, Req Request, Res Response] struct {
	ctx  context.Context
	bidi BidiClientMerge[Req, Res]

	err      error
	curr     *ingestv1.ProfileSets
	currIdx  int
	keep     []bool
	keepSent bool // keepSent is true if we have sent the keep request to the ingester.

	currentProfile *ProfileWithLabels

	response keepResponse
}

// NewMergeIterator return a new iterator that stream profiles and allows to filter them using `Keep` to keep
// only a subset of the profiles for an aggregation result.
// Merging or querying profiles sample values is expensive, we only merge the sample of the profiles that are kept.
// On creating the iterator, we send a request to ingesters to fetch the first batch.
func NewMergeIterator[
	R any,
	Req Request,
	Res Response,
](ctx context.Context, r responseFromIngesters[BidiClientMerge[Req, Res]],
) *mergeIterator[R, Req, Res] {
	it := &mergeIterator[R, Req, Res]{
		bidi:           r.response,
		keepSent:       true, // at the start we don't send a keep request.
		ctx:            ctx,
		currentProfile: &ProfileWithLabels{IngesterAddr: r.addr},
		currIdx:        -1,
		response: keepResponse{
			MergeProfilesStacktracesRequest: &ingestv1.MergeProfilesStacktracesRequest{},
			MergeProfilesLabelsRequest:      &ingestv1.MergeProfilesLabelsRequest{},
			MergeProfilesPprofRequest:       &ingestv1.MergeProfilesPprofRequest{},
		},
	}
	it.fetchBatch()
	return it
}

func (s *mergeIterator[R, Req, Res]) Next() bool {
	if s.curr == nil || len(s.curr.Profiles) == 0 {
		return false
	}
	if s.currIdx >= len(s.curr.Profiles)-1 {
		if !s.keepSent {
			var err error
			switch bidi := (s.bidi).(type) {
			case BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]:
				s.response.MergeProfilesStacktracesRequest.Profiles = s.keep
				err = bidi.Send(s.response.MergeProfilesStacktracesRequest)
			case BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]:
				s.response.MergeProfilesLabelsRequest.Profiles = s.keep
				err = bidi.Send(s.response.MergeProfilesLabelsRequest)
			case BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]:
				s.response.MergeProfilesPprofRequest.Profiles = s.keep
				err = bidi.Send(s.response.MergeProfilesPprofRequest)
			}
			if err != nil {
				s.err = err
				return false
			}
		}
		s.fetchBatch()
		if s.curr == nil || len(s.curr.Profiles) == 0 {
			return false
		}
		s.currIdx = 0
		s.currentProfile.Timestamp = s.curr.Profiles[s.currIdx].Timestamp
		s.currentProfile.Labels = s.curr.LabelsSets[s.curr.Profiles[s.currIdx].LabelIndex].Labels
		return true
	}
	s.currIdx++
	s.currentProfile.Timestamp = s.curr.Profiles[s.currIdx].Timestamp
	s.currentProfile.Labels = s.curr.LabelsSets[s.curr.Profiles[s.currIdx].LabelIndex].Labels
	return true
}

func (s *mergeIterator[R, Req, Res]) fetchBatch() {
	var selectedProfiles *ingestv1.ProfileSets
	switch bidi := (s.bidi).(type) {
	case BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]:
		res, err := bidi.Receive()
		if err != nil {
			s.err = err
			return
		}
		selectedProfiles = res.SelectedProfiles
	case BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]:
		res, err := bidi.Receive()
		if err != nil {
			s.err = err
			return
		}
		selectedProfiles = res.SelectedProfiles
	case BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]:
		res, err := bidi.Receive()
		if err != nil {
			s.err = err
			return
		}
		selectedProfiles = res.SelectedProfiles
	}
	s.curr = selectedProfiles
	if s.curr == nil {
		return
	}
	if len(s.curr.Profiles) > cap(s.keep) {
		s.keep = make([]bool, len(s.curr.Profiles))
	}
	s.keep = s.keep[:len(s.curr.Profiles)]
	// reset selections to none
	for i := range s.keep {
		s.keep[i] = false
	}
	s.keepSent = false
}

func (s *mergeIterator[R, Req, Res]) Keep() {
	s.keep[s.currIdx] = true
}

func (s *mergeIterator[R, Req, Res]) At() *ProfileWithLabels {
	return s.currentProfile
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

// skipDuplicates iterates through the iterator and skip duplicates.
func skipDuplicates(ctx context.Context, its []MergeIterator) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "skipDuplicates")
	defer span.Finish()
	var errors multierror.MultiError
	tree := loser.New(its,
		&ProfileWithLabels{
			Timestamp: math.MaxInt64,
		},
		func(s MergeIterator) *ProfileWithLabels {
			return s.At()
		},
		func(p1, p2 *ProfileWithLabels) bool {
			if p1.Timestamp == p2.Timestamp {
				return phlaremodel.CompareLabelPairs(p1.Labels, p2.Labels) < 0
			}
			return p1.Timestamp < p2.Timestamp
		},
		func(s MergeIterator) {
			if err := s.Close(); err != nil {
				errors.Add(err)
			}
		})

	defer tree.Close()
	duplicates := 0
	total := 0
	previousTs := int64(-1)
	previousLabels := phlaremodel.Labels{}
	for tree.Next() {
		next := tree.Winner()
		profile := next.At()
		total++
		if previousTs != profile.Timestamp || phlaremodel.CompareLabelPairs(previousLabels, profile.Labels) != 0 {
			previousTs = profile.Timestamp
			previousLabels = profile.Labels
			next.Keep()
			continue
		}
		duplicates++
	}
	span.LogFields(otlog.Int("duplicates", duplicates))
	span.LogFields(otlog.Int("total", total))

	return errors.Err()
}

type stacktraces struct {
	locations []string
	value     int64
}

// selectMergeStacktraces selects the  profile from each ingester by deduping them and request merges of stacktraces of them.
func selectMergeStacktraces(ctx context.Context, responses []responseFromIngesters[clientpool.BidiClientMergeProfilesStacktraces]) ([]stacktraces, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "selectMergeStacktraces")
	defer span.Finish()

	mergeResults := make([]MergeResult[*ingestv1.MergeProfilesStacktracesResult], len(responses))
	iters := make([]MergeIterator, len(responses))
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp responseFromIngesters[clientpool.BidiClientMergeProfilesStacktraces]) {
			defer wg.Done()
			it := NewMergeIterator[*ingestv1.MergeProfilesStacktracesResult](
				ctx, responseFromIngesters[BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]]{
					addr:     resp.addr,
					response: resp.response,
				})
			iters[i] = it
			mergeResults[i] = it
		}(i, resp)
	}
	wg.Wait()

	if err := skipDuplicates(ctx, iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([]*ingestv1.MergeProfilesStacktracesResult, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
		iter := iter
		g.Go(util.RecoverPanic(func() error {
			result, err := iter.Result()
			if err != nil {
				return err
			}
			s.Do(func() {
				results = append(results, result)
			})
			return nil
		}))
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
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp responseFromIngesters[clientpool.BidiClientMergeProfilesPprof]) {
			defer wg.Done()
			it := NewMergeIterator[[]byte](
				ctx, responseFromIngesters[BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]]{
					addr:     resp.addr,
					response: resp.response,
				})
			iters[i] = it
			mergeResults[i] = it
		}(i, resp)
	}
	wg.Wait()

	if err := skipDuplicates(ctx, iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([]*profile.Profile, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
		iter := iter
		g.Go(util.RecoverPanic(func() error {
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
		}))
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
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp responseFromIngesters[clientpool.BidiClientMergeProfilesLabels]) {
			defer wg.Done()
			it := NewMergeIterator[[]*typesv1.Series](
				ctx, responseFromIngesters[BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]]{
					addr:     resp.addr,
					response: resp.response,
				})
			iters[i] = it
			mergeResults[i] = it
		}(i, resp)
	}
	wg.Wait()

	if err := skipDuplicates(ctx, iters); err != nil {
		return nil, err
	}

	// Collects the results in parallel.
	results := make([][]*typesv1.Series, 0, len(iters))
	s := lo.Synchronize()
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
		iter := iter
		g.Go(util.RecoverPanic(func() error {
			result, err := iter.Result()
			if err != nil {
				return err
			}
			s.Do(func() {
				results = append(results, result)
			})
			return nil
		}))
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
