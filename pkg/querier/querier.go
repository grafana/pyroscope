package querier

import (
	"context"
	"flag"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/querier/vcs"
	"github.com/grafana/pyroscope/pkg/storegateway"
	pmath "github.com/grafana/pyroscope/pkg/util/math"
	"github.com/grafana/pyroscope/pkg/util/spanlogger"
	"github.com/grafana/pyroscope/pkg/validation"
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

	vcsv1connect.VCSServiceHandler

	storageBucket        phlareobj.Bucket
	tenantConfigProvider phlareobj.TenantConfigProvider
}

// TODO(kolesnikovae): For backwards compatibility.
// Should be removed in the next release.
//
// The default value should never be used in practice:
// querier frontend sets the limit.
const maxNodesDefault = int64(2048)

type NewQuerierParams struct {
	Cfg             Config
	StoreGatewayCfg storegateway.Config
	Overrides       *validation.Overrides
	StorageBucket   phlareobj.Bucket
	CfgProvider     phlareobj.TenantConfigProvider
	IngestersRing   ring.ReadRing
	PoolFactory     ring_client.PoolFactory
	Reg             prometheus.Registerer
	Logger          log.Logger
	ClientOptions   []connect.ClientOption
}

func New(params *NewQuerierParams) (*Querier, error) {
	// disable gzip compression for querier-ingester communication as most of payload are not benefit from it.
	clientsMetrics := promauto.With(params.Reg).NewGauge(prometheus.GaugeOpts{
		Namespace: "pyroscope",
		Name:      "querier_ingester_clients",
		Help:      "The current number of ingester clients.",
	})

	// if a storage bucket is configured we need to create a store gateway querier
	var storeGatewayQuerier *StoreGatewayQuerier
	var err error
	if params.StorageBucket != nil {
		storeGatewayQuerier, err = newStoreGatewayQuerier(
			params.StoreGatewayCfg,
			params.PoolFactory,
			params.Overrides,
			log.With(params.Logger, "component", "store-gateway-querier"),
			params.Reg,
			params.ClientOptions...)
		if err != nil {
			return nil, err
		}
	}

	q := &Querier{
		cfg:    params.Cfg,
		logger: params.Logger,
		ingesterQuerier: NewIngesterQuerier(
			clientpool.NewIngesterPool(params.Cfg.PoolConfig, params.IngestersRing, params.PoolFactory, clientsMetrics, params.Logger, params.ClientOptions...),
			params.IngestersRing,
		),
		storeGatewayQuerier:  storeGatewayQuerier,
		VCSServiceHandler:    vcs.New(params.Logger),
		storageBucket:        params.StorageBucket,
		tenantConfigProvider: params.CfgProvider,
	}

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

	lblReq := connect.NewRequest(&typesv1.LabelValuesRequest{
		Start:    req.Msg.Start,
		End:      req.Msg.End,
		Matchers: []string{"{}"},
		Name:     phlaremodel.LabelNameProfileType,
	})

	lblRes, err := q.LabelValues(ctx, lblReq)
	if err != nil {
		return nil, err
	}

	var profileTypes []*typesv1.ProfileType

	for _, profileTypeStr := range lblRes.Msg.Names {
		profileType, err := phlaremodel.ParseProfileTypeSelector(profileTypeStr)
		if err != nil {
			return nil, err
		}
		profileTypes = append(profileTypes, profileType)
	}

	sort.Slice(profileTypes, func(i, j int) bool {
		return profileTypes[i].ID < profileTypes[j].ID
	})

	return connect.NewResponse(&querierv1.ProfileTypesResponse{
		ProfileTypes: profileTypes,
	}), nil
}

func (q *Querier) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelValues")
	defer sp.Finish()

	_, hasTimeRange := phlaremodel.GetTimeRange(req.Msg)
	sp.LogFields(
		otlog.Bool("legacy_request", !hasTimeRange),
		otlog.String("name", req.Msg.Name),
		otlog.String("matchers", strings.Join(req.Msg.Matchers, ",")),
		otlog.Int64("start", req.Msg.Start),
		otlog.Int64("end", req.Msg.End),
	)

	if q.storeGatewayQuerier == nil || !hasTimeRange {
		responses, err := q.labelValuesFromIngesters(ctx, req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&typesv1.LabelValuesResponse{
			Names: uniqueSortedStrings(responses),
		}), nil
	}

	storeQueries := splitQueryToStores(model.Time(req.Msg.Start), model.Time(req.Msg.End), model.Now(), q.cfg.QueryStoreAfter, nil)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}
	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	var responses []ResponseFromReplica[[]string]
	var lock sync.Mutex
	group, ctx := errgroup.WithContext(ctx)

	if storeQueries.ingester.shouldQuery {
		group.Go(func() error {
			ir, err := q.labelValuesFromIngesters(ctx, storeQueries.ingester.LabelValuesRequest(req.Msg))
			if err != nil {
				return err
			}

			lock.Lock()
			responses = append(responses, ir...)
			lock.Unlock()
			return nil
		})
	}

	if storeQueries.storeGateway.shouldQuery {
		group.Go(func() error {
			ir, err := q.labelValuesFromStoreGateway(ctx, storeQueries.storeGateway.LabelValuesRequest(req.Msg))
			if err != nil {
				return err
			}

			lock.Lock()
			responses = append(responses, ir...)
			lock.Unlock()
			return nil
		})
	}

	err := group.Wait()
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

	_, hasTimeRange := phlaremodel.GetTimeRange(req.Msg)
	sp.LogFields(
		otlog.Bool("legacy_request", !hasTimeRange),
		otlog.String("matchers", strings.Join(req.Msg.Matchers, ",")),
		otlog.Int64("start", req.Msg.Start),
		otlog.Int64("end", req.Msg.End),
	)

	if q.storeGatewayQuerier == nil || !hasTimeRange {
		responses, err := q.labelNamesFromIngesters(ctx, req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&typesv1.LabelNamesResponse{
			Names: uniqueSortedStrings(responses),
		}), nil
	}

	storeQueries := splitQueryToStores(model.Time(req.Msg.Start), model.Time(req.Msg.End), model.Now(), q.cfg.QueryStoreAfter, nil)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}
	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	var responses []ResponseFromReplica[[]string]
	var lock sync.Mutex
	group, ctx := errgroup.WithContext(ctx)

	if storeQueries.ingester.shouldQuery {
		group.Go(func() error {
			ir, err := q.labelNamesFromIngesters(ctx, storeQueries.ingester.LabelNamesRequest(req.Msg))
			if err != nil {
				return err
			}

			lock.Lock()
			responses = append(responses, ir...)
			lock.Unlock()
			return nil
		})
	}

	if storeQueries.storeGateway.shouldQuery {
		group.Go(func() error {
			ir, err := q.labelNamesFromStoreGateway(ctx, storeQueries.storeGateway.LabelNamesRequest(req.Msg))
			if err != nil {
				return err
			}

			lock.Lock()
			responses = append(responses, ir...)
			lock.Unlock()
			return nil
		})
	}

	err := group.Wait()
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&typesv1.LabelNamesResponse{
		Names: uniqueSortedStrings(responses),
	}), nil
}

func (q *Querier) blockSelect(ctx context.Context, start, end model.Time) (blockPlan, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "blockSelect")
	defer sp.Finish()

	sp.LogFields(
		otlog.String("start", start.Time().String()),
		otlog.String("end", end.Time().String()),
	)

	ingesterReq := &ingestv1.BlockMetadataRequest{
		Start: int64(start),
		End:   int64(end),
	}

	results := newReplicasPerBlockID(q.logger)

	// get first all blocks from store gateways, as they should be querier with a priority and also are the only ones containing duplicated blocks because of replication
	if q.storeGatewayQuerier != nil {
		res, err := q.blockSelectFromStoreGateway(ctx, ingesterReq)
		if err != nil {
			return nil, err
		}

		results.add(res, storeGatewayInstance)
	}

	if q.ingesterQuerier != nil {
		res, err := q.blockSelectFromIngesters(ctx, ingesterReq)
		if err != nil {
			return nil, err
		}
		results.add(res, ingesterInstance)
	}

	return results.blockPlan(ctx), nil
}

func (q *Querier) Series(ctx context.Context, req *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Series")
	defer sp.Finish()

	_, hasTimeRange := phlaremodel.GetTimeRange(req.Msg)
	sp.LogFields(
		otlog.Bool("legacy_request", !hasTimeRange),
		otlog.String("matchers", strings.Join(req.Msg.Matchers, ",")),
		otlog.String("label_names", strings.Join(req.Msg.LabelNames, ",")),
		otlog.Int64("start", req.Msg.Start),
		otlog.Int64("end", req.Msg.End),
	)
	// no store gateways configured so just query the ingesters
	if q.storeGatewayQuerier == nil || !hasTimeRange {
		responses, err := q.seriesFromIngesters(ctx, &ingestv1.SeriesRequest{
			Matchers:   req.Msg.Matchers,
			LabelNames: req.Msg.LabelNames,
			Start:      req.Msg.Start,
			End:        req.Msg.End,
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

	storeQueries := splitQueryToStores(model.Time(req.Msg.Start), model.Time(req.Msg.End), model.Now(), q.cfg.QueryStoreAfter, nil)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}
	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	var responses []ResponseFromReplica[[]*typesv1.Labels]
	var lock sync.Mutex
	group, ctx := errgroup.WithContext(ctx)

	if storeQueries.ingester.shouldQuery {
		group.Go(func() error {
			ir, err := q.seriesFromIngesters(ctx, storeQueries.ingester.SeriesRequest(req.Msg))
			if err != nil {
				return err
			}

			lock.Lock()
			responses = append(responses, ir...)
			lock.Unlock()
			return nil
		})
	}

	if storeQueries.storeGateway.shouldQuery {
		group.Go(func() error {
			ir, err := q.seriesFromStoreGateway(ctx, storeQueries.storeGateway.SeriesRequest(req.Msg))
			if err != nil {
				return err
			}

			lock.Lock()
			responses = append(responses, ir...)
			lock.Unlock()
			return nil
		})
	}

	err := group.Wait()
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
			},
		),
	}), nil
}

// FIXME(kolesnikovae): The method is never used and should be removed.
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

	fd, err := phlaremodel.NewFlamegraphDiff(leftTree, rightTree, maxNodesDefault)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	return connect.NewResponse(&querierv1.DiffResponse{
		Flamegraph: fd,
	}), nil
}

func (q *Querier) GetProfileStats(ctx context.Context, req *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "GetProfileStats")
	defer sp.Finish()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) (*typesv1.GetProfileStatsResponse, error) {
		response, err := ic.GetProfileStats(childCtx, connect.NewRequest(&typesv1.GetProfileStatsRequest{}))
		if err != nil {
			return nil, err
		}
		return response.Msg, nil
	})
	if err != nil {
		return nil, err
	}

	response := &typesv1.GetProfileStatsResponse{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}
	for _, r := range responses {
		response.DataIngested = response.DataIngested || r.response.DataIngested
		if r.response.OldestProfileTime < response.OldestProfileTime {
			response.OldestProfileTime = r.response.OldestProfileTime
		}
		if r.response.NewestProfileTime > response.NewestProfileTime {
			response.NewestProfileTime = r.response.NewestProfileTime
		}
	}

	if q.storageBucket != nil {
		tenantId, err := tenant.TenantID(ctx)
		if err != nil {
			return nil, err
		}
		index, err := bucketindex.ReadIndex(ctx, q.storageBucket, tenantId, q.tenantConfigProvider, q.logger)
		if err != nil && !errors.Is(err, bucketindex.ErrIndexNotFound) {
			return nil, err
		}
		if index != nil && len(index.Blocks) > 0 {
			// assuming blocks are ordered by time in ascending order
			// ignoring deleted blocks as we only need the overall time range of blocks
			minTime := index.Blocks[0].MinTime.Time().UnixMilli()
			if minTime < response.OldestProfileTime {
				response.OldestProfileTime = minTime
			}
			maxTime := index.Blocks[len(index.Blocks)-1].MaxTime.Time().UnixMilli()
			if maxTime > response.NewestProfileTime {
				response.NewestProfileTime = maxTime
			}
			response.DataIngested = true
		}
	}

	return connect.NewResponse(response), nil
}

func (q *Querier) AnalyzeQuery(ctx context.Context, req *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "AnalyzeQuery")
	defer sp.Finish()

	ingesterReq := &ingestv1.BlockMetadataRequest{
		Start: req.Msg.Start,
		End:   req.Msg.End,
	}

	resultsIngesters := newReplicasPerBlockID(q.logger)
	blockSelectIngesters, err := q.blockSelectFromIngesters(ctx, ingesterReq)
	if err != nil {
		return nil, err
	}
	resultsIngesters.add(blockSelectIngesters, ingesterInstance)

	resultsStoreGateways := newReplicasPerBlockID(q.logger)
	blockSelectStoreGateways, err := q.blockSelectFromStoreGateway(ctx, ingesterReq)
	if err != nil {
		return nil, err
	}
	resultsStoreGateways.add(blockSelectStoreGateways, storeGatewayInstance)

	joinedResults := newReplicasPerBlockID(q.logger)
	joinedResults.add(blockSelectStoreGateways, storeGatewayInstance)
	joinedResults.add(blockSelectIngesters, ingesterInstance)
	plan := joinedResults.blockPlan(ctx)

	storeGatewayReplicationSet, err := q.storeGatewayQuerier.ring.GetReplicationSetForOperation(readNoExtend)
	if err != nil {
		return nil, err
	}
	ingesterReplicationSet, err := q.ingesterQuerier.ring.GetReplicationSetForOperation(readNoExtend)
	if err != nil {
		return nil, err
	}
	storeGatewayQueryScope := &querierv1.QueryScope{
		ComponentType:  "long-term-storage",
		ComponentCount: 0,
	}
	ingesterQueryScope := &querierv1.QueryScope{
		ComponentType:  "short-term-storage",
		ComponentCount: 0,
	}
	ingesterBlockUlids := make([]string, 0)
	storeGatewayBlockUlids := make([]string, 0)
	for replica, blockHints := range plan {
		if len(blockHints.Ulids) == 0 {
			continue
		}
		if storeGatewayReplicationSet.Includes(replica) && ingesterReplicationSet.Includes(replica) { // -target=all
			for _, ulid := range blockHints.Ulids {
				if resultsIngesters.contains(ulid) {
					ingesterQueryScope.ComponentCount += 1
					ingesterQueryScope.NumBlocks += 1
					ingesterBlockUlids = append(ingesterBlockUlids, ulid)
				} else if resultsStoreGateways.contains(ulid) {
					storeGatewayQueryScope.ComponentCount += 1
					storeGatewayQueryScope.NumBlocks += 1
					storeGatewayBlockUlids = append(storeGatewayBlockUlids, ulid)
				}
			}
		} else if storeGatewayReplicationSet.Includes(replica) {
			storeGatewayQueryScope.ComponentCount += 1
			storeGatewayQueryScope.NumBlocks += uint64(len(blockHints.Ulids))
			storeGatewayBlockUlids = append(storeGatewayBlockUlids, blockHints.Ulids...)
		} else if ingesterReplicationSet.Includes(replica) {
			ingesterQueryScope.ComponentCount += 1
			ingesterQueryScope.NumBlocks += uint64(len(blockHints.Ulids))
			ingesterBlockUlids = append(ingesterBlockUlids, blockHints.Ulids...)
		}
	}

	var responses []ResponseFromReplica[*ingestv1.GetBlockStatsResponse]
	responses, err = forAllPlannedIngesters(ctx, q.ingesterQuerier, plan, func(ctx context.Context, iq IngesterQueryClient, hint *ingestv1.Hints) (*ingestv1.GetBlockStatsResponse, error) {
		stats, err := iq.GetBlockStats(ctx, connect.NewRequest(&ingestv1.GetBlockStatsRequest{Ulids: ingesterBlockUlids}))
		if err != nil {
			return nil, err
		}
		return stats.Msg, err
	})
	for _, r := range responses {
		for _, stats := range r.response.BlockStats {
			ingesterQueryScope.NumSeries += stats.NumSeries
			ingesterQueryScope.NumProfiles += stats.NumProfiles
			ingesterQueryScope.NumSamples += stats.NumSamples
			ingesterQueryScope.IndexBytes += stats.IndexBytes
			ingesterQueryScope.ProfileBytes += stats.ProfilesBytes
			ingesterQueryScope.SymbolBytes += stats.SymbolsBytes
		}
	}

	tenantId, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, err
	}
	responses, err = forAllPlannedStoreGateways(ctx, tenantId, q.storeGatewayQuerier, plan, func(ctx context.Context, sq StoreGatewayQueryClient, hint *ingestv1.Hints) (*ingestv1.GetBlockStatsResponse, error) {
		stats, err := sq.GetBlockStats(ctx, connect.NewRequest(&ingestv1.GetBlockStatsRequest{Ulids: storeGatewayBlockUlids}))
		return stats.Msg, err
	})
	for _, r := range responses {
		for _, stats := range r.response.BlockStats {
			storeGatewayQueryScope.NumSeries += stats.NumSeries
			storeGatewayQueryScope.NumProfiles += stats.NumProfiles
			storeGatewayQueryScope.NumSamples += stats.NumSamples
			storeGatewayQueryScope.IndexBytes += stats.IndexBytes
			storeGatewayQueryScope.ProfileBytes += stats.ProfilesBytes
			storeGatewayQueryScope.SymbolBytes += stats.SymbolsBytes
		}
	}
	totalBytes := ingesterQueryScope.IndexBytes +
		ingesterQueryScope.ProfileBytes +
		ingesterQueryScope.SymbolBytes +
		storeGatewayQueryScope.IndexBytes +
		storeGatewayQueryScope.ProfileBytes +
		storeGatewayQueryScope.SymbolBytes

	res := &querierv1.AnalyzeQueryResponse{
		QueryValidationErrors: nil,
		QueryScopes:           []*querierv1.QueryScope{ingesterQueryScope, storeGatewayQueryScope},
		QueryImpact: &querierv1.QueryImpact{
			Type:               querierv1.QueryImpactType_MEDIUM, // TODO
			TotalBytesRead:     totalBytes,
			EstimatedTimeNanos: 0, // TODO
		},
	}

	return connect.NewResponse(res), nil
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

func (q *Querier) SelectMergeSpanProfile(ctx context.Context, req *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeSpanProfile")
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

	t, err := q.selectSpanProfile(ctx, req.Msg)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&querierv1.SelectMergeSpanProfileResponse{
		Flamegraph: phlaremodel.NewFlameGraph(t, req.Msg.GetMaxNodes()),
	}), nil
}

func isEndpointNotExistingErr(err error) bool {
	if err == nil {
		return false
	}

	var cerr *connect.Error
	// unwrap all intermediate connect errors
	for errors.As(err, &cerr) {
		err = cerr.Unwrap()
	}
	return err.Error() == "405 Method Not Allowed"
}

func (q *Querier) selectTree(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest) (*phlaremodel.Tree, error) {
	// determine the block hints
	plan, err := q.blockSelect(ctx, model.Time(req.Start), model.Time(req.End))
	if isEndpointNotExistingErr(err) {
		level.Warn(spanlogger.FromContext(ctx, q.logger)).Log(
			"msg", "block select not supported on at least one component, fallback to use full dataset",
			"err", err,
		)
		plan = nil
	} else if err != nil {
		return nil, fmt.Errorf("error during block select: %w", err)
	}

	// no store gateways configured so just query the ingesters
	if q.storeGatewayQuerier == nil {
		return q.selectTreeFromIngesters(ctx, req, plan)
	}

	storeQueries := splitQueryToStores(model.Time(req.Start), model.Time(req.End), model.Now(), q.cfg.QueryStoreAfter, plan)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}

	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	if plan == nil && !storeQueries.ingester.shouldQuery {
		return q.selectTreeFromStoreGateway(ctx, storeQueries.storeGateway.MergeStacktracesRequest(req), plan)
	}
	if plan == nil && !storeQueries.storeGateway.shouldQuery {
		return q.selectTreeFromIngesters(ctx, storeQueries.ingester.MergeStacktracesRequest(req), plan)
	}

	g, ctx := errgroup.WithContext(ctx)
	var ingesterTree, storegatewayTree *phlaremodel.Tree
	g.Go(func() error {
		var err error
		ingesterTree, err = q.selectTreeFromIngesters(ctx, storeQueries.ingester.MergeStacktracesRequest(req), plan)
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		var err error
		storegatewayTree, err = q.selectTreeFromStoreGateway(ctx, storeQueries.storeGateway.MergeStacktracesRequest(req), plan)
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
		Request: &ingestv1.SelectProfilesRequest{
			Type:          profileType,
			LabelSelector: req.LabelSelector,
			Start:         int64(sq.start),
			End:           int64(sq.end),
			Aggregation:   req.Aggregation,
		},
		By:                 req.GroupBy,
		StackTraceSelector: req.StackTraceSelector,
	}
}

func (sq storeQuery) MergeSpanProfileRequest(req *querierv1.SelectMergeSpanProfileRequest) *querierv1.SelectMergeSpanProfileRequest {
	return &querierv1.SelectMergeSpanProfileRequest{
		Start:         int64(sq.start),
		End:           int64(sq.end),
		ProfileTypeID: req.ProfileTypeID,
		LabelSelector: req.LabelSelector,
		SpanSelector:  req.SpanSelector,
		MaxNodes:      req.MaxNodes,
	}
}

func (sq storeQuery) MergeProfileRequest(req *querierv1.SelectMergeProfileRequest) *querierv1.SelectMergeProfileRequest {
	return &querierv1.SelectMergeProfileRequest{
		ProfileTypeID:      req.ProfileTypeID,
		LabelSelector:      req.LabelSelector,
		Start:              int64(sq.start),
		End:                int64(sq.end),
		MaxNodes:           req.MaxNodes,
		StackTraceSelector: req.StackTraceSelector,
	}
}

func (sq storeQuery) SeriesRequest(req *querierv1.SeriesRequest) *ingestv1.SeriesRequest {
	return &ingestv1.SeriesRequest{
		Start:      int64(sq.start),
		End:        int64(sq.end),
		Matchers:   req.Matchers,
		LabelNames: req.LabelNames,
	}
}

func (sq storeQuery) LabelNamesRequest(req *typesv1.LabelNamesRequest) *typesv1.LabelNamesRequest {
	return &typesv1.LabelNamesRequest{
		Matchers: req.Matchers,
		Start:    int64(sq.start),
		End:      int64(sq.end),
	}
}

func (sq storeQuery) LabelValuesRequest(req *typesv1.LabelValuesRequest) *typesv1.LabelValuesRequest {
	return &typesv1.LabelValuesRequest{
		Name:     req.Name,
		Matchers: req.Matchers,
		Start:    int64(sq.start),
		End:      int64(sq.end),
	}
}

func (sq storeQuery) ProfileTypesRequest(req *querierv1.ProfileTypesRequest) *ingestv1.ProfileTypesRequest {
	return &ingestv1.ProfileTypesRequest{
		Start: int64(sq.start),
		End:   int64(sq.end),
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
func splitQueryToStores(start, end model.Time, now model.Time, queryStoreAfter time.Duration, plan blockPlan) (queries storeQueries) {
	if plan != nil {
		// if we have a plan we can use it to split the query, we retain the original start and end time as we want to query the full range for those particular blocks selected.
		queries.queryStoreAfter = 0
		queries.ingester = storeQuery{shouldQuery: true, start: start, end: end}
		queries.storeGateway = storeQuery{shouldQuery: true, start: start, end: end}
		return queries
	}

	queries.queryStoreAfter = queryStoreAfter
	cutOff := now.Add(-queryStoreAfter)
	if start.Before(cutOff) {
		queries.storeGateway = storeQuery{shouldQuery: true, start: start, end: pmath.Min(cutOff, end)}
	}
	if end.After(cutOff) {
		queries.ingester = storeQuery{shouldQuery: true, start: pmath.Max(cutOff, start), end: end}
		// Note that the ranges must not overlap.
		if queries.storeGateway.shouldQuery {
			queries.ingester.start++
		}
	}
	return queries
}

func (q *Querier) SelectMergeProfile(ctx context.Context, req *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[googlev1.Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeProfile")
	sp.SetTag("start", model.Time(req.Msg.Start).Time().String()).
		SetTag("end", model.Time(req.Msg.End).Time().String()).
		SetTag("selector", req.Msg.LabelSelector).
		SetTag("max_nodes", req.Msg.GetMaxNodes()).
		SetTag("profile_type", req.Msg.ProfileTypeID)
	defer sp.Finish()

	profile, err := q.selectProfile(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	profile.DurationNanos = model.Time(req.Msg.End).UnixNano() - model.Time(req.Msg.Start).UnixNano()
	profile.TimeNanos = model.Time(req.Msg.End).UnixNano()
	return connect.NewResponse(profile), nil
}

func (q *Querier) selectProfile(ctx context.Context, req *querierv1.SelectMergeProfileRequest) (*googlev1.Profile, error) {
	// determine the block hints
	plan, err := q.blockSelect(ctx, model.Time(req.Start), model.Time(req.End))
	if isEndpointNotExistingErr(err) {
		level.Warn(spanlogger.FromContext(ctx, q.logger)).Log(
			"msg", "block select not supported on at least one component, fallback to use full dataset",
			"err", err,
		)
		plan = nil
	} else if err != nil {
		return nil, fmt.Errorf("error during block select: %w", err)
	}

	// no store gateways configured so just query the ingesters
	if q.storeGatewayQuerier == nil {
		return q.selectProfileFromIngesters(ctx, req, plan)
	}

	storeQueries := splitQueryToStores(model.Time(req.Start), model.Time(req.End), model.Now(), q.cfg.QueryStoreAfter, plan)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}

	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	if plan == nil && !storeQueries.ingester.shouldQuery {
		return q.selectProfileFromStoreGateway(ctx, storeQueries.storeGateway.MergeProfileRequest(req), plan)
	}
	if plan == nil && !storeQueries.storeGateway.shouldQuery {
		return q.selectProfileFromIngesters(ctx, storeQueries.ingester.MergeProfileRequest(req), plan)
	}

	g, ctx := errgroup.WithContext(ctx)
	var lock sync.Mutex
	var merge pprof.ProfileMerge
	g.Go(func() error {
		ingesterProfile, err := q.selectProfileFromIngesters(ctx, storeQueries.ingester.MergeProfileRequest(req), plan)
		if err != nil {
			return err
		}
		lock.Lock()
		defer lock.Unlock()
		return merge.Merge(ingesterProfile)
	})
	g.Go(func() error {
		storegatewayProfile, err := q.selectProfileFromStoreGateway(ctx, storeQueries.storeGateway.MergeProfileRequest(req), plan)
		if err != nil {
			return err
		}
		lock.Lock()
		defer lock.Unlock()
		return merge.Merge(storegatewayProfile)
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return merge.Profile(), nil
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

	// determine the block hints
	plan, err := q.blockSelect(ctx, model.Time(req.Msg.Start), model.Time(req.Msg.End))
	if isEndpointNotExistingErr(err) {
		level.Warn(spanlogger.FromContext(ctx, q.logger)).Log(
			"msg", "block select not supported on at least one component, fallback to use full dataset",
			"err", err,
		)
		plan = nil
	} else if err != nil {
		return nil, fmt.Errorf("error during block select: %w", err)
	}

	responses, err := q.selectSeries(ctx, req, plan)
	if err != nil {
		return nil, err
	}

	it, err := selectMergeSeries(ctx, req.Msg.Aggregation, responses)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	result := rangeSeries(it, req.Msg.Start, req.Msg.End, stepMs, req.Msg.Aggregation)
	if it.Err() != nil {
		return nil, connect.NewError(connect.CodeInternal, it.Err())
	}

	return connect.NewResponse(&querierv1.SelectSeriesResponse{
		Series: result,
	}), nil
}

func (q *Querier) selectSeries(ctx context.Context, req *connect.Request[querierv1.SelectSeriesRequest], plan map[string]*ingestv1.BlockHints) ([]ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels], error) {
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
				Aggregation:   req.Msg.Aggregation,
			},
			By:                 req.Msg.GroupBy,
			StackTraceSelector: req.Msg.StackTraceSelector,
		}, plan)
	}

	storeQueries := splitQueryToStores(model.Time(start), model.Time(req.Msg.End), model.Now(), q.cfg.QueryStoreAfter, plan)

	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]

	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}

	// todo in parallel

	if storeQueries.ingester.shouldQuery {
		ir, err := q.selectSeriesFromIngesters(ctx, storeQueries.ingester.MergeSeriesRequest(req.Msg, profileType), plan)
		if err != nil {
			return nil, err
		}
		responses = append(responses, ir...)
	}

	if storeQueries.storeGateway.shouldQuery {
		ir, err := q.selectSeriesFromStoreGateway(ctx, storeQueries.storeGateway.MergeSeriesRequest(req.Msg, profileType), plan)
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
func rangeSeries(it iter.Iterator[ProfileValue], start, end, step int64, aggregation *typesv1.TimeSeriesAggregationType) []*typesv1.Series {
	defer it.Close()
	seriesMap := make(map[uint64]*typesv1.Series)
	aggregators := make(map[uint64]TimeSeriesAggregator)

	if !it.Next() {
		return nil
	}

	// advance from the start to the end, adding each step results to the map.
Outer:
	for currentStep := start; currentStep <= end; currentStep += step {
		for {
			aggregator, ok := aggregators[it.At().LabelsHash]
			if !ok {
				aggregator = NewTimeSeriesAggregator(aggregation)
				aggregators[it.At().LabelsHash] = aggregator
			}
			if it.At().Ts > currentStep {
				if !aggregator.IsEmpty() {
					series := seriesMap[it.At().LabelsHash]
					series.Points = append(series.Points, aggregator.GetAndReset())
				}
				break // no more profiles for the currentStep
			}
			// find or create series
			series, ok := seriesMap[it.At().LabelsHash]
			if !ok {
				seriesMap[it.At().LabelsHash] = &typesv1.Series{
					Labels: it.At().Lbs,
					Points: []*typesv1.Point{},
				}
				aggregator.Add(currentStep, it.At().Value)
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Aggregate point if it is in the current step.
			if aggregator.GetTimestamp() == currentStep {
				aggregator.Add(currentStep, it.At().Value)
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Next step is missing
			if !aggregator.IsEmpty() {
				series.Points = append(series.Points, aggregator.GetAndReset())
			}
			aggregator.Add(currentStep, it.At().Value)
			if !it.Next() {
				break Outer
			}
		}
	}
	for lblHash, aggregator := range aggregators {
		if !aggregator.IsEmpty() {
			seriesMap[lblHash].Points = append(seriesMap[lblHash].Points, aggregator.GetAndReset())
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

func (q *Querier) selectSpanProfile(ctx context.Context, req *querierv1.SelectMergeSpanProfileRequest) (*phlaremodel.Tree, error) {
	// determine the block hints
	plan, err := q.blockSelect(ctx, model.Time(req.Start), model.Time(req.End))
	if isEndpointNotExistingErr(err) {
		level.Warn(spanlogger.FromContext(ctx, q.logger)).Log(
			"msg", "block select not supported on at least one component, fallback to use full dataset",
			"err", err,
		)
		plan = nil
	} else if err != nil {
		return nil, fmt.Errorf("error during block select: %w", err)
	}

	// no store gateways configured so just query the ingesters
	if q.storeGatewayQuerier == nil {
		return q.selectSpanProfileFromIngesters(ctx, req, plan)
	}

	storeQueries := splitQueryToStores(model.Time(req.Start), model.Time(req.End), model.Now(), q.cfg.QueryStoreAfter, plan)
	if !storeQueries.ingester.shouldQuery && !storeQueries.storeGateway.shouldQuery {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start and end time are outside of the ingester and store gateway retention"))
	}

	storeQueries.Log(level.Debug(spanlogger.FromContext(ctx, q.logger)))

	if plan == nil && !storeQueries.ingester.shouldQuery {
		return q.selectSpanProfileFromStoreGateway(ctx, storeQueries.storeGateway.MergeSpanProfileRequest(req), plan)
	}
	if plan == nil && !storeQueries.storeGateway.shouldQuery {
		return q.selectSpanProfileFromIngesters(ctx, storeQueries.ingester.MergeSpanProfileRequest(req), plan)
	}

	g, ctx := errgroup.WithContext(ctx)
	var ingesterTree, storegatewayTree *phlaremodel.Tree
	g.Go(func() error {
		var err error
		ingesterTree, err = q.selectSpanProfileFromIngesters(ctx, storeQueries.ingester.MergeSpanProfileRequest(req), plan)
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		var err error
		storegatewayTree, err = q.selectSpanProfileFromStoreGateway(ctx, storeQueries.storeGateway.MergeSpanProfileRequest(req), plan)
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

type TimeSeriesAggregator interface {
	Add(ts int64, value float64)
	GetAndReset() *typesv1.Point
	IsEmpty() bool
	GetTimestamp() int64
}

func NewTimeSeriesAggregator(aggregation *typesv1.TimeSeriesAggregationType) TimeSeriesAggregator {
	if aggregation == nil {
		return &sumTimeSeriesAggregator{
			ts: -1,
		}
	}
	if *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE {
		return &avgTimeSeriesAggregator{
			ts: -1,
		}
	}
	return &sumTimeSeriesAggregator{
		ts: -1,
	}
}

type sumTimeSeriesAggregator struct {
	ts  int64
	sum float64
}

func (a *sumTimeSeriesAggregator) Add(ts int64, value float64) {
	a.ts = ts
	a.sum += value
}

func (a *sumTimeSeriesAggregator) GetAndReset() *typesv1.Point {
	tsCopy := a.ts
	sumCopy := a.sum
	a.ts = -1
	a.sum = 0
	return &typesv1.Point{
		Timestamp: tsCopy,
		Value:     sumCopy,
	}
}

func (a *sumTimeSeriesAggregator) IsEmpty() bool {
	return a.ts == -1
}

func (a *sumTimeSeriesAggregator) GetTimestamp() int64 {
	return a.ts
}

type avgTimeSeriesAggregator struct {
	ts    int64
	sum   float64
	count int64
}

func (a *avgTimeSeriesAggregator) Add(ts int64, value float64) {
	a.ts = ts
	a.sum += value
	a.count++
}

func (a *avgTimeSeriesAggregator) GetAndReset() *typesv1.Point {
	avg := a.sum / float64(a.count)
	tsCopy := a.ts
	a.ts = -1
	a.sum = 0
	a.count = 0
	return &typesv1.Point{
		Timestamp: tsCopy,
		Value:     avg,
	}
}

func (a *avgTimeSeriesAggregator) IsEmpty() bool {
	return a.ts == -1
}

func (a *avgTimeSeriesAggregator) GetTimestamp() int64 {
	return a.ts
}
