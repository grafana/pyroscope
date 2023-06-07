package querier

import (
	"context"
	"flag"
	"sort"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/grafana/mimir/pkg/util/spanlogger"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/clientpool"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/math"
)

type Config struct {
	PoolConfig      clientpool.PoolConfig `yaml:"pool_config,omitempty"`
	QueryStoreAfter time.Duration         `yaml:"query_store_after" category:"advanced"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.PoolConfig.RegisterFlagsWithPrefix("querier", fs)
	fs.DurationVar(&cfg.QueryStoreAfter, "querier.query-store-after", 4*time.Hour, "The time after which a metric should be queried from storage and not just ingesters. 0 means all queries are sent to store. If this option is enabled, the time range of the query sent to the store-gateway will be manipulated to ensure the query end is not more recent than 'now - query-store-after'.")
}

type Querier struct {
	services.Service
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	cfg    Config
	logger log.Logger

	ingesterQuerier     *IngesterQuerier
	storeGatewayQuerier *StoreGatewayQuerier
}

const maxNodesDefault = int64(2048)

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, storeGatewayQuerier *StoreGatewayQuerier, reg prometheus.Registerer, logger log.Logger, clientsOptions ...connect.ClientOption) (*Querier, error) {
	// disable gzip compression for querier-ingester communication as most of payload are not benefit from it.
	clientsOptions = append(clientsOptions, connect.WithAcceptCompression("gzip", nil, nil))
	clientsMetrics := promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Namespace: "pyroscope",
		Name:      "querier_ingester_clients",
		Help:      "The current number of ingester clients.",
	})

	q := &Querier{
		cfg:    cfg,
		logger: logger,
		ingesterQuerier: NewIngesterQuerier(
			clientpool.NewIngesterPool(cfg.PoolConfig, ingestersRing, factory, clientsMetrics, logger, clientsOptions...),
			ingestersRing,
		),
		storeGatewayQuerier: storeGatewayQuerier,
	}
	var err error
	svcs := []services.Service{q.ingesterQuerier.pool}
	if storeGatewayQuerier != nil {
		svcs = append(svcs, storeGatewayQuerier)
	}
	// should we watch for the ring module status ?
	q.subservices, err = services.NewManager(svcs...)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	q.subservicesWatcher = services.NewFailureWatcher()
	q.subservicesWatcher.WatchManager(q.subservices)
	q.Service = services.NewBasicService(q.starting, q.running, q.stopping)
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
		return errors.Wrap(err, "querier subservice failed")
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

func (q *Querier) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelValues")
	defer func() {
		sp.LogFields(
			otlog.String("name", req.Msg.Name),
		)
		sp.Finish()
	}()
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelValues(childCtx, connect.NewRequest(&typesv1.LabelValuesRequest{
			Name:     req.Msg.Name,
			Matchers: req.Msg.Matchers,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&typesv1.LabelValuesResponse{
		Names: uniqueSortedStrings(responses),
	}), nil
}

func (q *Querier) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelNames")
	defer sp.Finish()
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelNames(childCtx, connect.NewRequest(&typesv1.LabelNamesRequest{
			Matchers: req.Msg.Matchers,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&typesv1.LabelNamesResponse{
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
			lo.FlatMap(responses, func(r ResponseFromReplica[[]*typesv1.Labels], _ int) []*typesv1.Labels {
				return r.response
			}),
			func(t *typesv1.Labels) uint64 {
				return phlaremodel.Labels(t.Labels).Hash()
			}),
	}), nil
}

func (q *Querier) Diff(ctx context.Context, req *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Diff")
	defer func() {
		sp.LogFields(
			otlog.String("leftStart", model.Time(req.Msg.Left.Start).Time().String()),
			otlog.String("leftEnd", model.Time(req.Msg.Left.End).Time().String()),
			// Assume are the same
			otlog.String("selector", req.Msg.Left.LabelSelector),
			otlog.String("profile_id", req.Msg.Left.ProfileTypeID),
		)
		sp.Finish()
	}()

	var leftTree, rightTree *phlaremodel.Tree
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		res, err := q.selectTree(gCtx, req.Msg.Left)
		if err != nil {
			return err
		}

		leftTree = res
		return nil
	})

	g.Go(func() error {
		res, err := q.selectTree(gCtx, req.Msg.Right)
		if err != nil {
			return err
		}
		rightTree = res
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	fd, err := phlaremodel.NewFlamegraphDiff(leftTree, rightTree, phlaremodel.MaxNodes)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	return connect.NewResponse(&querierv1.DiffResponse{
		Flamegraph: fd,
	}), nil
}

func (q *Querier) SelectMergeStacktraces(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeStacktraces")
	level.Info(spanlogger.FromContext(ctx, q.logger)).Log(
		"start", model.Time(req.Msg.Start).Time().String(),
		"end", model.Time(req.Msg.End).Time().String(),
		"selector", req.Msg.LabelSelector,
		"profile_id", req.Msg.ProfileTypeID,
	)
	defer func() {
		sp.Finish()
	}()

	if req.Msg.MaxNodes == nil || *req.Msg.MaxNodes == 0 {
		mn := maxNodesDefault
		req.Msg.MaxNodes = &mn
	}

	t, err := q.selectTree(ctx, req.Msg)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Flamegraph: phlaremodel.NewFlameGraph(t, req.Msg.GetMaxNodes()),
	}), nil
}

func (q *Querier) selectTree(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest) (*phlaremodel.Tree, error) {
	// no store gateways configured so just query the ingesters
	if q.storeGatewayQuerier == nil {
		return q.selectTreeFromIngesters(ctx, req)
	}

	storeQueries := splitQueryToStores(model.Time(req.Start), model.Time(req.End), model.Now(), q.cfg.QueryStoreAfter)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}

	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	if !storeQueries.ingester.shouldQuery {
		return q.selectTreeFromStoreGateway(ctx, storeQueries.storeGateway.MergeStacktracesRequest(req))
	}
	if !storeQueries.storeGateway.shouldQuery {
		return q.selectTreeFromIngesters(ctx, storeQueries.ingester.MergeStacktracesRequest(req))
	}

	g, ctx := errgroup.WithContext(ctx)
	var ingesterTree, storegatewayTree *phlaremodel.Tree
	g.Go(func() error {
		var err error
		ingesterTree, err = q.selectTreeFromIngesters(ctx, storeQueries.ingester.MergeStacktracesRequest(req))
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		var err error
		storegatewayTree, err = q.selectTreeFromStoreGateway(ctx, storeQueries.storeGateway.MergeStacktracesRequest(req))
		if err != nil {
			return err
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	storegatewayTree.Merge(ingesterTree)
	return storegatewayTree, nil
}

type storeQuery struct {
	start, end  model.Time
	shouldQuery bool
}

func (sq storeQuery) MergeStacktracesRequest(req *querierv1.SelectMergeStacktracesRequest) *querierv1.SelectMergeStacktracesRequest {
	return &querierv1.SelectMergeStacktracesRequest{
		Start:         int64(sq.start),
		End:           int64(sq.end),
		LabelSelector: req.LabelSelector,
		ProfileTypeID: req.ProfileTypeID,
		MaxNodes:      req.MaxNodes,
	}
}

func (sq storeQuery) MergeSeriesRequest(req *querierv1.SelectSeriesRequest, profileType *typesv1.ProfileType) *ingestv1.MergeProfilesLabelsRequest {
	return &ingestv1.MergeProfilesLabelsRequest{
		By: req.GroupBy,
		Request: &ingestv1.SelectProfilesRequest{
			Type:          profileType,
			LabelSelector: req.LabelSelector,
			Start:         int64(sq.start),
			End:           int64(sq.end),
		},
	}
}

type storeQueries struct {
	ingester, storeGateway storeQuery
	queryStoreAfter        time.Duration
}

func (sq storeQueries) Log(logger log.Logger) {
	logger.Log(
		"msg", "storeQueries",
		"queryStoreAfter", sq.queryStoreAfter.String(),
		"ingester", sq.ingester.shouldQuery,
		"ingester.start", sq.ingester.start.Time().Format(time.RFC3339Nano), "ingester.end", sq.ingester.end.Time().Format(time.RFC3339Nano),
		"store-gateway", sq.storeGateway.shouldQuery,
		"store-gateway.start", sq.storeGateway.start.Time().Format(time.RFC3339Nano), "store-gateway.end", sq.storeGateway.end.Time().Format(time.RFC3339Nano),
	)
}

// splitQueryToStores splits the query into ingester and store gateway queries using the given cut off time.
// todo(ctovena): Later we should try to deduplicate blocks between ingesters and store gateways (prefer) and simply query both
func splitQueryToStores(start, end model.Time, now model.Time, queryStoreAfter time.Duration) (queries storeQueries) {
	queries.queryStoreAfter = queryStoreAfter
	cutOff := now.Add(-queryStoreAfter)
	if start.Before(cutOff) {
		queries.storeGateway = storeQuery{shouldQuery: true, start: start, end: math.Min(cutOff, end)}
	}
	if end.After(cutOff) {
		queries.ingester = storeQuery{shouldQuery: true, start: math.Max(cutOff, start), end: end}
		// Note that the ranges must not overlap.
		if queries.storeGateway.shouldQuery {
			queries.ingester.start++
		}
	}
	return queries
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
	_, err = parser.ParseMetricSelector(req.Msg.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesPprof, error) {
		return ic.MergeProfilesPprof(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesPprofRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.Msg.LabelSelector,
					Start:         req.Msg.Start,
					End:           req.Msg.End,
					Type:          profileType,
				},
			})
		}))
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

	_, err := parser.ParseMetricSelector(req.Msg.LabelSelector)
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses, err := q.selectSeries(ctx, req)
	if err != nil {
		return nil, err
	}

	it, err := selectMergeSeries(ctx, responses)
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

func (q *Querier) selectSeries(ctx context.Context, req *connect.Request[querierv1.SelectSeriesRequest]) ([]ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels], error) {
	stepMs := time.Duration(req.Msg.Step * float64(time.Second)).Milliseconds()
	sort.Strings(req.Msg.GroupBy)

	// we need to request profile from start - step to end since start is inclusive.
	// The first step starts at start-step to start.
	start := req.Msg.Start - stepMs

	profileType, err := phlaremodel.ParseProfileTypeSelector(req.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if q.storeGatewayQuerier == nil {
		return q.selectSeriesFromIngesters(ctx, &ingestv1.MergeProfilesLabelsRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: req.Msg.LabelSelector,
				Start:         start,
				End:           req.Msg.End,
				Type:          profileType,
			},
			By: req.Msg.GroupBy,
		})
	}

	storeQueries := splitQueryToStores(model.Time(start), model.Time(req.Msg.End), model.Now(), q.cfg.QueryStoreAfter)

	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]

	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}

	// todo in parallel
	if storeQueries.ingester.shouldQuery {
		ir, err := q.selectSeriesFromIngesters(ctx, storeQueries.ingester.MergeSeriesRequest(req.Msg, profileType))
		if err != nil {
			return nil, err
		}
		responses = append(responses, ir...)
	}

	if storeQueries.storeGateway.shouldQuery {
		ir, err := q.selectSeriesFromStoreGateway(ctx, storeQueries.storeGateway.MergeSeriesRequest(req.Msg, profileType))
		if err != nil {
			return nil, err
		}
		responses = append(responses, ir...)
	}
	return responses, nil
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

func uniqueSortedStrings(responses []ResponseFromReplica[[]string]) []string {
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
