package querier

import (
	"context"
	"flag"
	"sort"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/ingester/clientpool"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
)

// todo: move to non global metrics.
var clients = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "phlare",
	Name:      "querier_ingester_clients",
	Help:      "The current number of ingester clients.",
})

type Config struct {
	PoolConfig      clientpool.PoolConfig `yaml:"pool_config,omitempty"`
	ExtraQueryDelay time.Duration         `yaml:"extra_query_delay,omitempty"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.PoolConfig.RegisterFlagsWithPrefix("querier", fs)
	fs.DurationVar(&cfg.ExtraQueryDelay, "querier.extra-query-delay", 0, "Time to wait before sending more than the minimum successful query requests.")
}

type Querier struct {
	services.Service
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	cfg    Config
	logger log.Logger

	ingestersRing   ring.ReadRing
	pool            *ring_client.Pool
	ingesterQuerier *IngesterQuerier
}

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, logger log.Logger, clientsOptions ...connect.ClientOption) (*Querier, error) {
	q := &Querier{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		pool:          clientpool.NewPool(cfg.PoolConfig, ingestersRing, factory, clients, logger, clientsOptions...),
	}
	var err error
	q.subservices, err = services.NewManager(q.pool)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	q.subservicesWatcher = services.NewFailureWatcher()
	q.subservicesWatcher.WatchManager(q.subservices)
	q.Service = services.NewBasicService(q.starting, q.running, q.stopping)
	q.ingesterQuerier = NewIngesterQuerier(q.pool, ingestersRing, cfg.ExtraQueryDelay)
	return q, nil
}

func (q *Querier) starting(ctx context.Context) error {
	return services.StartManagerAndAwaitHealthy(ctx, q.subservices)
}

func (q *Querier) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-q.subservicesWatcher.Chan():
		return errors.Wrap(err, "distributor subservice failed")
	}
}

func (q *Querier) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), q.subservices)
}

func (q *Querier) ProfileTypes(ctx context.Context, req *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "ProfileTypes")
	defer sp.Finish()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]*typesv1.ProfileType, error) {
		res, err := ic.ProfileTypes(childCtx, connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
		if err != nil {
			return nil, err
		}
		return res.Msg.ProfileTypes, nil
	})
	if err != nil {
		return nil, err
	}
	var profileTypeIDs []string
	profileTypes := make(map[string]*typesv1.ProfileType)
	for _, response := range responses {
		for _, profileType := range response.response {
			if _, ok := profileTypes[profileType.ID]; !ok {
				profileTypeIDs = append(profileTypeIDs, profileType.ID)
				profileTypes[profileType.ID] = profileType
			}
		}
	}
	sort.Strings(profileTypeIDs)
	result := &querierv1.ProfileTypesResponse{
		ProfileTypes: make([]*typesv1.ProfileType, 0, len(profileTypes)),
	}
	for _, id := range profileTypeIDs {
		result.ProfileTypes = append(result.ProfileTypes, profileTypes[id])
	}
	return connect.NewResponse(result), nil
}

func (q *Querier) LabelValues(ctx context.Context, req *connect.Request[querierv1.LabelValuesRequest]) (*connect.Response[querierv1.LabelValuesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelValues")
	defer func() {
		sp.LogFields(
			otlog.String("name", req.Msg.Name),
		)
		sp.Finish()
	}()
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelValues(childCtx, connect.NewRequest(&ingestv1.LabelValuesRequest{
			Name: req.Msg.Name,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&querierv1.LabelValuesResponse{
		Names: uniqueSortedStrings(responses),
	}), nil
}

func (q *Querier) LabelNames(ctx context.Context, req *connect.Request[querierv1.LabelNamesRequest]) (*connect.Response[querierv1.LabelNamesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelNames")
	defer sp.Finish()
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelNames(childCtx, connect.NewRequest(&ingestv1.LabelNamesRequest{}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&querierv1.LabelNamesResponse{
		Names: uniqueSortedStrings(responses),
	}), nil
}

func (q *Querier) Series(ctx context.Context, req *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Series")
	defer func() {
		sp.LogFields(
			otlog.String("matchers", strings.Join(req.Msg.Matchers, ",")),
		)
		sp.Finish()
	}()
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]*typesv1.Labels, error) {
		res, err := ic.Series(childCtx, connect.NewRequest(&ingestv1.SeriesRequest{
			Matchers: req.Msg.Matchers,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.LabelsSet, nil
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&querierv1.SeriesResponse{
		LabelsSet: lo.UniqBy(
			lo.FlatMap(responses, func(r responseFromIngesters[[]*typesv1.Labels], _ int) []*typesv1.Labels {
				return r.response
			}),
			func(t *typesv1.Labels) uint64 {
				return phlaremodel.Labels(t.Labels).Hash()
			}),
	}), nil
}

func (q *Querier) SelectMergeStacktraces(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeStacktraces")
	defer func() {
		sp.LogFields(
			otlog.String("start", model.Time(req.Msg.Start).Time().String()),
			otlog.String("end", model.Time(req.Msg.End).Time().String()),
			otlog.String("selector", req.Msg.LabelSelector),
			otlog.String("profile_id", req.Msg.ProfileTypeID),
		)
		sp.Finish()
	}()

	profileType, err := phlaremodel.ParseProfileTypeSelector(req.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(_ context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesStacktraces, error) {
		// we plan to use those streams to merge profiles
		// so we use the main context here otherwise will be canceled
		return ic.MergeProfilesStacktraces(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(func() error {
			return r.response.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.Msg.LabelSelector,
					Start:         req.Msg.Start,
					End:           req.Msg.End,
					Type:          profileType,
				},
			})
		})
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	st, err := selectMergeStacktraces(gCtx, responses)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Flamegraph: NewFlameGraph(newTree(st)),
	}), nil
}

func (q *Querier) SelectMergeProfile(ctx context.Context, req *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[googlev1.Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeProfile")
	defer func() {
		sp.LogFields(
			otlog.String("start", model.Time(req.Msg.Start).Time().String()),
			otlog.String("end", model.Time(req.Msg.End).Time().String()),
			otlog.String("selector", req.Msg.LabelSelector),
			otlog.String("profile_id", req.Msg.ProfileTypeID),
		)
		sp.Finish()
	}()

	profileType, err := phlaremodel.ParseProfileTypeSelector(req.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(_ context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesPprof, error) {
		// we plan to use those streams to merge profiles
		// so we use the main context here otherwise will be canceled
		return ic.MergeProfilesPprof(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(func() error {
			return r.response.Send(&ingestv1.MergeProfilesPprofRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.Msg.LabelSelector,
					Start:         req.Msg.Start,
					End:           req.Msg.End,
					Type:          profileType,
				},
			})
		})
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	profile, err := selectMergePprofProfile(gCtx, profileType, responses)
	if err != nil {
		return nil, err
	}
	profile.DurationNanos = model.Time(req.Msg.End).UnixNano() - model.Time(req.Msg.Start).UnixNano()
	profile.TimeNanos = model.Time(req.Msg.End).UnixNano()
	return connect.NewResponse(profile), nil
}

func (q *Querier) SelectSeries(ctx context.Context, req *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSeries")
	defer func() {
		sp.LogFields(
			otlog.String("start", model.Time(req.Msg.Start).Time().String()),
			otlog.String("end", model.Time(req.Msg.End).Time().String()),
			otlog.String("selector", req.Msg.LabelSelector),
			otlog.String("profile_id", req.Msg.ProfileTypeID),
			otlog.String("group_by", strings.Join(req.Msg.GroupBy, ",")),
			otlog.Float64("step", req.Msg.Step),
		)
		sp.Finish()
	}()
	profileType, err := phlaremodel.ParseProfileTypeSelector(req.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if req.Msg.Start > req.Msg.End {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start must be before end"))
	}

	if req.Msg.Step == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("step must be non-zero"))
	}

	stepMs := time.Duration(req.Msg.Step * float64(time.Second)).Milliseconds()
	// we need to request profile from start - step to end since start is inclusive.
	// The first step starts at start-step to start.
	start := req.Msg.Start - stepMs
	sort.Strings(req.Msg.GroupBy)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(_ context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesLabels, error) {
		return ic.MergeProfilesLabels(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(func() error {
			return r.response.Send(&ingestv1.MergeProfilesLabelsRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.Msg.LabelSelector,
					Start:         start,
					End:           req.Msg.End,
					Type:          profileType,
				},
				By: req.Msg.GroupBy,
			})
		})
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	it, err := selectMergeSeries(gCtx, responses)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	result := rangeSeries(it, req.Msg.Start, req.Msg.End, stepMs)
	if it.Err() != nil {
		return nil, connect.NewError(connect.CodeInternal, it.Err())
	}

	return connect.NewResponse(&querierv1.SelectSeriesResponse{
		Series: result,
	}), nil
}

// rangeSeries aggregates profiles into series.
// Series contains points spaced by step from start to end.
// Profiles from the same step are aggregated into one point.
func rangeSeries(it iter.Iterator[ProfileValue], start, end, step int64) []*typesv1.Series {
	defer it.Close()
	seriesMap := make(map[uint64]*typesv1.Series)

	if !it.Next() {
		return nil
	}
	// advance from the start to the end, adding each step results to the map.
Outer:
	for currentStep := start; currentStep <= end; currentStep += step {
		for {
			if it.At().Ts > currentStep {
				break // no more profiles for the currentStep
			}
			// find or create series
			series, ok := seriesMap[it.At().LabelsHash]
			if !ok {
				seriesMap[it.At().LabelsHash] = &typesv1.Series{
					Labels: it.At().Lbs,
					Points: []*typesv1.Point{
						{Value: it.At().Value, Timestamp: currentStep},
					},
				}
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Aggregate point if it is in the current step.
			if series.Points[len(series.Points)-1].Timestamp == currentStep {
				series.Points[len(series.Points)-1].Value += it.At().Value
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Next step is missing
			series.Points = append(series.Points, &typesv1.Point{
				Value:     it.At().Value,
				Timestamp: currentStep,
			})
			if !it.Next() {
				break Outer
			}
		}
	}
	series := lo.Values(seriesMap)
	sort.Slice(series, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(series[i].Labels, series[j].Labels) < 0
	})
	return series
}

func uniqueSortedStrings(responses []responseFromIngesters[[]string]) []string {
	total := 0
	for _, r := range responses {
		total += len(r.response)
	}
	unique := make(map[string]struct{}, total)
	result := make([]string, 0, total)
	for _, r := range responses {
		for _, elem := range r.response {
			if _, ok := unique[elem]; !ok {
				unique[elem] = struct{}{}
				result = append(result, elem)
			}
		}
	}
	sort.Strings(result)
	return result
}
