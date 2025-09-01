package querier

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

type ProfileWithLabels struct {
	Timestamp    int64
	Fingerprint  uint64
	IngesterAddr string
	phlaremodel.Labels
}

type BidiClientMerge[Req any, Res any] interface {
	Send(Req) error
	Receive() (Res, error)
	CloseRequest() error
	CloseResponse() error
}

type Request interface {
	*ingestv1.MergeProfilesStacktracesRequest |
		*ingestv1.MergeProfilesLabelsRequest |
		*ingestv1.MergeProfilesPprofRequest |
		*ingestv1.MergeSpanProfileRequest
}

type Response interface {
	*ingestv1.MergeProfilesStacktracesResponse |
		*ingestv1.MergeProfilesLabelsResponse |
		*ingestv1.MergeProfilesPprofResponse |
		*ingestv1.MergeSpanProfileResponse
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
	*ingestv1.MergeSpanProfileRequest
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
](ctx context.Context, r ResponseFromReplica[BidiClientMerge[Req, Res]],
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
			MergeSpanProfileRequest:         &ingestv1.MergeSpanProfileRequest{},
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
			case BidiClientMerge[*ingestv1.MergeSpanProfileRequest, *ingestv1.MergeSpanProfileResponse]:
				s.response.MergeSpanProfileRequest.Profiles = s.keep
				err = bidi.Send(s.response.MergeSpanProfileRequest)
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
		s.setCurrentProfile()
		return true
	}
	s.currIdx++
	s.setCurrentProfile()
	return true
}

func (s *mergeIterator[R, Req, Res]) setCurrentProfile() {
	p := s.curr.Profiles[s.currIdx]
	s.currentProfile.Timestamp = p.Timestamp
	if len(s.curr.LabelsSets) > 0 {
		s.currentProfile.Labels = s.curr.LabelsSets[p.LabelIndex].Labels
	}
	if len(s.curr.Fingerprints) > 0 {
		s.currentProfile.Fingerprint = s.curr.Fingerprints[p.LabelIndex]
	}
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
	case BidiClientMerge[*ingestv1.MergeSpanProfileRequest, *ingestv1.MergeSpanProfileResponse]:
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
	res, err := s.bidi.Receive()
	if err != nil {
		s.err = err
		return *new(R), err
	}
	switch result := any(res).(type) {
	case *ingestv1.MergeProfilesStacktracesResponse:
		return any(result.Result).(R), nil
	case *ingestv1.MergeProfilesLabelsResponse:
		return any(result.Series).(R), nil
	case *ingestv1.MergeProfilesPprofResponse:
		return any(result.Result).(R), nil
	case *ingestv1.MergeSpanProfileResponse:
		return any(result.Result).(R), nil
	default:
		return *new(R), fmt.Errorf("unexpected response type %T", result)
	}
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
			return p1.Timestamp <= p2.Timestamp
		},
		func(s MergeIterator) {
			if err := s.Close(); err != nil {
				errors.Add(err)
			}
		})

	defer tree.Close()
	// We rely on the fact that profiles are ordered by timestamp.
	// In order to deduplicate profiles, we only keep the first profile
	// with a given fingerprint for a given timestamp.
	fingerprints := newTimestampedFingerprints()
	duplicates := 0
	total := 0
	for tree.Next() {
		next := tree.Winner()
		profile := next.At()
		total++
		fingerprint := profile.Fingerprint
		if fingerprint == 0 && len(profile.Labels) > 0 {
			fingerprint = profile.Hash()
		}
		if fingerprints.keep(profile.Timestamp, fingerprint) {
			next.Keep()
			continue
		}
		duplicates++
	}
	span.LogFields(otlog.Int("duplicates", duplicates))
	span.LogFields(otlog.Int("total", total))
	if err := tree.Err(); err != nil {
		errors.Add(err)
	}

	return errors.Err()
}

func newTimestampedFingerprints() *timestampedFingerprints {
	return &timestampedFingerprints{
		timestamp:    math.MaxInt64,
		fingerprints: make(map[uint64]struct{}),
	}
}

type timestampedFingerprints struct {
	timestamp    int64
	fingerprints map[uint64]struct{}
}

// keep reports whether the profile has unique fingerprint for the timestamp.
func (p *timestampedFingerprints) keep(ts int64, fingerprint uint64) bool {
	if p.timestamp != ts {
		p.reset(ts, fingerprint)
		return true
	}
	return !p.fingerprintSeen(fingerprint)
}

func (p *timestampedFingerprints) reset(ts int64, fingerprint uint64) {
	p.timestamp = ts
	clear(p.fingerprints)
	p.fingerprints[fingerprint] = struct{}{}
}

func (p *timestampedFingerprints) fingerprintSeen(fingerprint uint64) (seen bool) {
	_, seen = p.fingerprints[fingerprint]
	if seen {
		return true
	}
	p.fingerprints[fingerprint] = struct{}{}
	return false
}

// selectMergeTree selects the  profile from each ingester by deduping them and
// returns merge of stacktrace samples represented as a tree.
func selectMergeTree(ctx context.Context, responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]) (*phlaremodel.Tree, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "selectMergeTree")
	defer span.Finish()

	mergeResults := make([]MergeResult[*ingestv1.MergeProfilesStacktracesResult], len(responses))
	iters := make([]MergeIterator, len(responses))
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]) {
			defer wg.Done()
			it := NewMergeIterator[*ingestv1.MergeProfilesStacktracesResult](
				ctx, ResponseFromReplica[BidiClientMerge[*ingestv1.MergeProfilesStacktracesRequest, *ingestv1.MergeProfilesStacktracesResponse]]{
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
	span.LogFields(otlog.String("msg", "collecting merge results"))
	g, _ := errgroup.WithContext(ctx)
	m := phlaremodel.NewTreeMerger()
	sm := phlaremodel.NewStackTraceMerger()
	for _, iter := range mergeResults {
		iter := iter
		g.Go(util.RecoverPanic(func() error {
			result, err := iter.Result()
			if err != nil || result == nil {
				return err
			}
			switch result.Format {
			default:
				return fmt.Errorf("unknown merge result format")
			case ingestv1.StacktracesMergeFormat_MERGE_FORMAT_STACKTRACES:
				sm.MergeStackTraces(result.Stacktraces, result.FunctionNames)
			case ingestv1.StacktracesMergeFormat_MERGE_FORMAT_TREE:
				err = m.MergeTreeBytes(result.TreeBytes)
			}
			return err
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if sm.Size() > 0 {
		// For backward compatibility: during a rollout, multiple formats
		// may coexist for some period of time (efficiency is not a concern).
		if err := m.MergeTreeBytes(sm.TreeBytes(-1)); err != nil {
			return nil, err
		}
	}

	span.LogFields(otlog.String("msg", "building tree"))
	return m.Tree(), nil
}

// selectMergePprofProfile selects the  profile from each ingester by deduping them and request merges of stacktraces in the pprof format.
func selectMergePprofProfile(ctx context.Context, ty *typesv1.ProfileType, responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesPprof]) (*googlev1.Profile, error) {
	mergeResults := make([]MergeResult[[]byte], len(responses))
	iters := make([]MergeIterator, len(responses))
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp ResponseFromReplica[clientpool.BidiClientMergeProfilesPprof]) {
			defer wg.Done()
			it := NewMergeIterator[[]byte](
				ctx, ResponseFromReplica[BidiClientMerge[*ingestv1.MergeProfilesPprofRequest, *ingestv1.MergeProfilesPprofResponse]]{
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

	span := opentracing.SpanFromContext(ctx)
	var pprofMerge pprof.ProfileMerge
	g, _ := errgroup.WithContext(ctx)
	for _, iter := range mergeResults {
		iter := iter
		g.Go(util.RecoverPanic(func() error {
			start := time.Now()
			result, err := iter.Result()
			if err != nil || result == nil {
				return err
			}
			if span != nil {
				span.LogFields(
					otlog.Int("profile_size", len(result)),
					otlog.Int64("took_ms", time.Since(start).Milliseconds()),
				)
			}
			var p googlev1.Profile
			if err = pprof.Unmarshal(result, &p); err != nil {
				return err
			}
			return pprofMerge.Merge(&p, true)
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	p := pprofMerge.Profile()
	if len(p.Sample) == 0 {
		pprof.SetProfileMetadata(p, ty, 0, 0)
	}
	return p, nil
}

// selectMergeSeries selects the  profile from each ingester by deduping them and request merges of total values.
func selectMergeSeries(ctx context.Context, aggregation *typesv1.TimeSeriesAggregationType, responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]) (iter.Iterator[phlaremodel.TimeSeriesValue], error) {
	mergeResults := make([]MergeResult[[]*typesv1.Series], len(responses))
	iters := make([]MergeIterator, len(responses))
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]) {
			defer wg.Done()
			it := NewMergeIterator[[]*typesv1.Series](
				ctx, ResponseFromReplica[BidiClientMerge[*ingestv1.MergeProfilesLabelsRequest, *ingestv1.MergeProfilesLabelsResponse]]{
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
			if err != nil || result == nil {
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
	var series = phlaremodel.MergeSeries(aggregation, results...)

	seriesIters := make([]iter.Iterator[phlaremodel.TimeSeriesValue], 0, len(series))
	for _, s := range series {
		s := s
		seriesIters = append(seriesIters, phlaremodel.NewSeriesIterator(s.Labels, s.Points))
	}
	return phlaremodel.NewMergeIterator(phlaremodel.TimeSeriesValue{Ts: math.MaxInt64}, false, seriesIters...), nil
}

// selectMergeSpanProfile selects the  profile from each ingester by deduping them and
// returns merge of stacktrace samples represented as a tree.
func selectMergeSpanProfile(ctx context.Context, responses []ResponseFromReplica[clientpool.BidiClientMergeSpanProfile]) (*phlaremodel.Tree, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "selectMergeSpanProfile")
	defer span.Finish()

	mergeResults := make([]MergeResult[*ingestv1.MergeSpanProfileResult], len(responses))
	iters := make([]MergeIterator, len(responses))
	var wg sync.WaitGroup
	for i, resp := range responses {
		wg.Add(1)
		go func(i int, resp ResponseFromReplica[clientpool.BidiClientMergeSpanProfile]) {
			defer wg.Done()
			it := NewMergeIterator[*ingestv1.MergeSpanProfileResult](
				ctx, ResponseFromReplica[BidiClientMerge[*ingestv1.MergeSpanProfileRequest, *ingestv1.MergeSpanProfileResponse]]{
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
	span.LogFields(otlog.String("msg", "collecting merge results"))
	g, _ := errgroup.WithContext(ctx)
	m := phlaremodel.NewTreeMerger()
	for _, iter := range mergeResults {
		iter := iter
		g.Go(util.RecoverPanic(func() error {
			result, err := iter.Result()
			if err != nil || result == nil {
				return err
			}
			return m.MergeTreeBytes(result.TreeBytes)
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	span.LogFields(otlog.String("msg", "building tree"))
	return m.Tree(), nil
}
