package querier

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/grafana/mimir/pkg/storegateway"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	ingesterv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/pkg/clientpool"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/util"
)

type StoreGatewayQueryClient interface {
	MergeProfilesStacktraces(context.Context) clientpool.BidiClientMergeProfilesStacktraces
	MergeProfilesLabels(ctx context.Context) clientpool.BidiClientMergeProfilesLabels
	MergeProfilesPprof(ctx context.Context) clientpool.BidiClientMergeProfilesPprof
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

func NewStoreGatewayQuerier(
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
	clientsOptions = append(clientsOptions, connect.WithAcceptCompression("gzip", nil, nil))
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

func (q *Querier) selectTreeFromStoreGateway(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest) (*phlaremodel.Tree, error) {
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

	responses, err := forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) (clientpool.BidiClientMergeProfilesStacktraces, error) {
		return ic.MergeProfilesStacktraces(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
				},
				MaxNodes: req.MaxNodes,
				// TODO(kolesnikovae): Max stacks.
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergeTree(gCtx, responses)
}

func (q *Querier) selectSeriesFromStoreGateway(ctx context.Context, req *ingesterv1.MergeProfilesLabelsRequest) ([]ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSeries StoreGateway")
	defer sp.Finish()
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	responses, err := forAllStoreGateways(ctx, tenantID, q.storeGatewayQuerier, func(ctx context.Context, ic StoreGatewayQueryClient) (clientpool.BidiClientMergeProfilesLabels, error) {
		return ic.MergeProfilesLabels(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, _ := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(req.CloneVT())
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}
