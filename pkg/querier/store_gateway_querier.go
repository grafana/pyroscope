package querier

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingesterv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/clientpool"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/storegateway"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
)

type StoreGatewayQueryClient interface {
	MergeProfilesStacktraces(context.Context) clientpool.BidiClientMergeProfilesStacktraces
	MergeProfilesLabels(ctx context.Context) clientpool.BidiClientMergeProfilesLabels
	MergeProfilesPprof(ctx context.Context) clientpool.BidiClientMergeProfilesPprof
	MergeSpanProfile(ctx context.Context) clientpool.BidiClientMergeSpanProfile
	ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error)
	LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error)
	LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error)
	Series(context.Context, *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error)
	BlockMetadata(ctx context.Context, req *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error)
	GetBlockStats(ctx context.Context, req *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error)
}

type StoreGatewayLimits interface {
	StoreGatewayTenantShardSize(userID string) int
}

type StoreGatewayQuerier struct {
	ring   ring.ReadRing
	pool   *ring_client.Pool
	limits StoreGatewayLimits

	services.Service
	// Subservices manager.
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher
}

func newStoreGatewayQuerier(
	gatewayCfg storegateway.Config,
	factory ring_client.PoolFactory,
	limits StoreGatewayLimits,
	logger log.Logger,
	reg prometheus.Registerer,
	clientsOptions ...connect.ClientOption,
) (*StoreGatewayQuerier, error) {
	storesRingCfg := gatewayCfg.ShardingRing.ToRingConfig()
	storesRingBackend, err := kv.NewClient(
		storesRingCfg.KVStore,
		ring.GetCodec(),
		kv.RegistererWithKVName(prometheus.WrapRegistererWithPrefix("pyroscope_", reg), "querier-store-gateway"),
		logger,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create store-gateway ring backend")
	}
	storesRing, err := ring.NewWithStoreClientAndStrategy(storesRingCfg, storegateway.RingNameForClient, storegateway.RingKey, storesRingBackend, ring.NewIgnoreUnhealthyInstancesReplicationStrategy(), prometheus.WrapRegistererWithPrefix("pyroscope_", reg), logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create store-gateway ring client")
	}
	// Disable compression for querier -> store-gateway connections
	clientsMetrics := promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Namespace:   "pyroscope",
		Name:        "storegateway_clients",
		Help:        "The current number of store-gateway clients in the pool.",
		ConstLabels: map[string]string{"client": "querier"},
	})
	pool := clientpool.NeStoreGatewayPool(storesRing, factory, clientsMetrics, logger, clientsOptions...)

	s := &StoreGatewayQuerier{
		ring:               storesRing,
		pool:               pool,
		limits:             limits,
		subservicesWatcher: services.NewFailureWatcher(),
	}
	s.subservices, err = services.NewManager(storesRing, pool)
	if err != nil {
		return nil, err
	}

	s.Service = services.NewBasicService(s.starting, s.running, s.stopping)

	return s, nil
}

func (s *StoreGatewayQuerier) starting(ctx context.Context) error {
	s.subservicesWatcher.WatchManager(s.subservices)

	if err := services.StartManagerAndAwaitHealthy(ctx, s.subservices); err != nil {
		return errors.Wrap(err, "unable to start store gateway querier set subservices")
	}

	return nil
}

func (s *StoreGatewayQuerier) running(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-s.subservicesWatcher.Chan():
			return errors.Wrap(err, "store gateway querier set subservice failed")
		}
	}
}

func (s *StoreGatewayQuerier) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), s.subservices)
}

// forAllStoreGateways runs f, in parallel, for all store-gateways that are part of the replication set for the given tenant.
func forAllStoreGateways[T any](ctx context.Context, tenantID string, storegatewayQuerier *StoreGatewayQuerier, f QueryReplicaFn[T, StoreGatewayQueryClient]) ([]ResponseFromReplica[T], error) {
	replicationSet, err := GetShuffleShardingSubring(storegatewayQuerier.ring, tenantID, storegatewayQuerier.limits).GetReplicationSetForOperation(storegateway.BlocksRead)
	if err != nil {
		return nil, err
	}

	return forGivenReplicationSet(ctx, func(addr string) (StoreGatewayQueryClient, error) {
		client, err := storegatewayQuerier.pool.GetClientFor(addr)
		if err != nil {
			return nil, err
		}
		return client.(StoreGatewayQueryClient), nil
	}, replicationSet, f)
}

// forAllPlannedStoreGatway runs f, in parallel, for all store-gateways part of the plan
func forAllPlannedStoreGateways[T any](ctx context.Context, _ string, storegatewayQuerier *StoreGatewayQuerier, plan map[string]*blockPlanEntry, f QueryReplicaWithHintsFn[T, StoreGatewayQueryClient]) ([]ResponseFromReplica[T], error) {
	replicationSet, err := storegatewayQuerier.ring.GetReplicationSetForOperation(readNoExtend)
	if err != nil {
		return nil, err
	}

	return forGivenPlan(ctx, plan, func(addr string) (StoreGatewayQueryClient, error) {
		client, err := storegatewayQuerier.pool.GetClientFor(addr)
		if err != nil {
			return nil, err
		}
		return client.(StoreGatewayQueryClient), nil
	}, replicationSet, f)
}

// GetShuffleShardingSubring returns the subring to be used for a given user. This function
// should be used both by store-gateway and querier in order to guarantee the same logic is used.
func GetShuffleShardingSubring(ring ring.ReadRing, userID string, limits StoreGatewayLimits) ring.ReadRing {
	shardSize := limits.StoreGatewayTenantShardSize(userID)

	// A shard size of 0 means shuffle sharding is disabled for this specific user,
	// so we just return the full ring so that blocks will be sharded across all store-gateways.
	if shardSize <= 0 {
		return ring
	}

	return ring.ShuffleShard(userID, shardSize)
}

func (q *Querier) selectTreeFromStoreGateway(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest, plan map[string]*blockPlanEntry) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectTree StoreGateway")
	defer sp.Finish()
	profileType, err := phlaremodel.ParseProfileTypeSelector(req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	_, err = parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]
	if plan != nil {
		responses, err = forAllPlannedStoreGateways(ctx, tenantID, q.storeGatewayQuerier, plan, func(ctx context.Context, ic StoreGatewayQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeProfilesStacktraces, error) {
			return ic.MergeProfilesStacktraces(ctx), nil
		})
	} else {
		responses, err = forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) (clientpool.BidiClientMergeProfilesStacktraces, error) {
			return ic.MergeProfilesStacktraces(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		blockHints, err := BlockHints(plan, r.addr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
					Hints:         &ingestv1.Hints{Block: blockHints},
				},
				MaxNodes: req.MaxNodes,
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergeTree(gCtx, responses)
}

func (q *Querier) selectProfileFromStoreGateway(ctx context.Context, req *querierv1.SelectMergeProfileRequest, plan map[string]*blockPlanEntry) (*googlev1.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectProfile StoreGateway")
	defer sp.Finish()
	profileType, err := phlaremodel.ParseProfileTypeSelector(req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	_, err = parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesPprof]
	if plan != nil {
		responses, err = forAllPlannedStoreGateways(ctx, tenantID, q.storeGatewayQuerier, plan, func(ctx context.Context, ic StoreGatewayQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeProfilesPprof, error) {
			return ic.MergeProfilesPprof(ctx), nil
		})
	} else {
		responses, err = forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) (clientpool.BidiClientMergeProfilesPprof, error) {
			return ic.MergeProfilesPprof(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		blockHints, err := BlockHints(plan, r.addr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesPprofRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
					Hints:         &ingestv1.Hints{Block: blockHints},
				},
				MaxNodes:           req.MaxNodes,
				StackTraceSelector: req.StackTraceSelector,
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergePprofProfile(gCtx, profileType, responses)
}

func (q *Querier) selectSeriesFromStoreGateway(ctx context.Context, req *ingesterv1.MergeProfilesLabelsRequest, plan map[string]*blockPlanEntry) ([]ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSeries StoreGateway")
	defer sp.Finish()
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]
	if plan != nil {
		responses, err = forAllPlannedStoreGateways(ctx, tenantID, q.storeGatewayQuerier, plan, func(ctx context.Context, ic StoreGatewayQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeProfilesLabels, error) {
			return ic.MergeProfilesLabels(ctx), nil
		})
	} else {
		responses, err = forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) (clientpool.BidiClientMergeProfilesLabels, error) {
			return ic.MergeProfilesLabels(ctx), nil
		})
	}

	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, _ := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		blockHints, err := BlockHints(plan, r.addr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		g.Go(util.RecoverPanic(func() error {
			req := req.CloneVT()
			req.Request.Hints = &ingestv1.Hints{Block: blockHints}
			return r.response.Send(req)
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) labelValuesFromStoreGateway(ctx context.Context, req *typesv1.LabelValuesRequest) ([]ResponseFromReplica[[]string], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelValues StoreGateway")
	defer sp.Finish()

	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	responses, err := forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) ([]string, error) {
		res, err := ic.LabelValues(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) labelNamesFromStoreGateway(ctx context.Context, req *typesv1.LabelNamesRequest) ([]ResponseFromReplica[[]string], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelNames StoreGateway")
	defer sp.Finish()

	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	responses, err := forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) ([]string, error) {
		res, err := ic.LabelNames(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) seriesFromStoreGateway(ctx context.Context, req *ingestv1.SeriesRequest) ([]ResponseFromReplica[[]*typesv1.Labels], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Series StoreGateway")
	defer sp.Finish()

	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	responses, err := forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) ([]*typesv1.Labels, error) {
		res, err := ic.Series(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.LabelsSet, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) selectSpanProfileFromStoreGateway(ctx context.Context, req *querierv1.SelectMergeSpanProfileRequest, plan map[string]*blockPlanEntry) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSpanProfile StoreGateway")
	defer sp.Finish()
	profileType, err := phlaremodel.ParseProfileTypeSelector(req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	_, err = parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var responses []ResponseFromReplica[clientpool.BidiClientMergeSpanProfile]
	if plan != nil {
		responses, err = forAllPlannedStoreGateways(ctx, tenantID, q.storeGatewayQuerier, plan, func(ctx context.Context, ic StoreGatewayQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeSpanProfile, error) {
			return ic.MergeSpanProfile(ctx), nil
		})
	} else {
		responses, err = forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) (clientpool.BidiClientMergeSpanProfile, error) {
			return ic.MergeSpanProfile(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		blockHints, err := BlockHints(plan, r.addr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeSpanProfileRequest{
				Request: &ingestv1.SelectSpanProfileRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
					SpanSelector:  req.SpanSelector,
					Hints:         &ingestv1.Hints{Block: blockHints},
				},
				MaxNodes: req.MaxNodes,
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergeSpanProfile(gCtx, responses)
}

func (q *Querier) blockSelectFromStoreGateway(ctx context.Context, req *ingestv1.BlockMetadataRequest) ([]ResponseFromReplica[[]*typesv1.BlockInfo], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "blockSelect StoreGateway")
	defer sp.Finish()

	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	responses, err := forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) ([]*typesv1.BlockInfo, error) {
		res, err := ic.BlockMetadata(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.Blocks, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}
