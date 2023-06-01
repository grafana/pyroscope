package storegateway

import (
	"context"
	"flag"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/grafana/mimir/pkg/storegateway"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	phlareobj "github.com/grafana/phlare/pkg/objstore"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/validation"
)

const (
	syncReasonInitial    = "initial"
	syncReasonPeriodic   = "periodic"
	syncReasonRingChange = "ring-change"

	// ringAutoForgetUnhealthyPeriods is how many consecutive timeout periods an unhealthy instance
	// in the ring will be automatically removed.
	ringAutoForgetUnhealthyPeriods = 10
)

// Validation errors.
var errInvalidTenantShardSize = errors.New("invalid tenant shard size, the value must be greater or equal to 0")

type Limits interface {
	storegateway.ShardingLimits
}

type StoreGateway struct {
	services.Service
	logger log.Logger

	gatewayCfg Config
	stores     *BucketStores

	// Ring used for sharding blocks.
	ringLifecycler *ring.BasicLifecycler
	ring           *ring.Ring

	// Subservices manager (ring, lifecycler)
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	bucketSync *prometheus.CounterVec
}

type Config struct {
	storegateway.Config
	BucketStoreConfig BucketStoreConfig `yaml:"bucket_store,omitempty"`
}

// RegisterFlags registers the Config flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	cfg.Config.RegisterFlags(f, logger)
	cfg.BucketStoreConfig.RegisterFlags(f, logger)
}

func (c *Config) Validate(limits validation.Limits) error {
	if err := c.BucketStoreConfig.Validate(util.Logger); err != nil {
		return errors.Wrap(err, "bucket store config")
	}
	if limits.StoreGatewayTenantShardSize < 0 {
		return errInvalidTenantShardSize
	}

	return nil
}

func NewStoreGateway(gatewayCfg Config, storageBucket phlareobj.Bucket, limits Limits, logger log.Logger, reg prometheus.Registerer) (*StoreGateway, error) {
	ringStore, err := kv.NewClient(
		gatewayCfg.ShardingRing.KVStore,
		ring.GetCodec(),
		kv.RegistererWithKVName(prometheus.WrapRegistererWithPrefix("pyroscope_", reg), "store-gateway"),
		logger,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create KV store client")
	}

	return newStoreGateway(gatewayCfg, storageBucket, ringStore, limits, logger, reg)
}

func newStoreGateway(gatewayCfg Config, storageBucket phlareobj.Bucket, ringStore kv.Client, limits Limits, logger log.Logger, reg prometheus.Registerer) (*StoreGateway, error) {
	var err error

	g := &StoreGateway{
		gatewayCfg: gatewayCfg,
		logger:     logger,
		bucketSync: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storegateway_bucket_sync_total",
			Help: "Total number of times the bucket sync operation triggered.",
		}, []string{"reason"}),
	}

	// Init metrics.
	g.bucketSync.WithLabelValues(syncReasonInitial)
	g.bucketSync.WithLabelValues(syncReasonPeriodic)
	g.bucketSync.WithLabelValues(syncReasonRingChange)

	// Init sharding strategy.
	var shardingStrategy ShardingStrategy

	lifecyclerCfg, err := gatewayCfg.ShardingRing.ToLifecyclerConfig(logger)
	if err != nil {
		return nil, errors.Wrap(err, "invalid ring lifecycler config")
	}

	// Define lifecycler delegates in reverse order (last to be called defined first because they're
	// chained via "next delegate").
	delegate := ring.BasicLifecyclerDelegate(ring.NewInstanceRegisterDelegate(ring.JOINING, storegateway.RingNumTokens))
	delegate = ring.NewLeaveOnStoppingDelegate(delegate, logger)
	delegate = ring.NewTokensPersistencyDelegate(gatewayCfg.ShardingRing.TokensFilePath, ring.JOINING, delegate, logger)
	delegate = ring.NewAutoForgetDelegate(ringAutoForgetUnhealthyPeriods*gatewayCfg.ShardingRing.HeartbeatTimeout, delegate, logger)

	g.ringLifecycler, err = ring.NewBasicLifecycler(lifecyclerCfg, storegateway.RingNameForServer, storegateway.RingKey, ringStore, delegate, logger, prometheus.WrapRegistererWithPrefix("cortex_", reg))
	if err != nil {
		return nil, errors.Wrap(err, "create ring lifecycler")
	}

	ringCfg := gatewayCfg.ShardingRing.ToRingConfig()
	g.ring, err = ring.NewWithStoreClientAndStrategy(ringCfg, storegateway.RingNameForServer, storegateway.RingKey, ringStore, ring.NewIgnoreUnhealthyInstancesReplicationStrategy(), prometheus.WrapRegistererWithPrefix("cortex_", reg), logger)
	if err != nil {
		return nil, errors.Wrap(err, "create ring client")
	}

	shardingStrategy = NewShuffleShardingStrategy(g.ring, lifecyclerCfg.ID, lifecyclerCfg.Addr, limits, logger)

	g.stores, err = NewBucketStores(gatewayCfg.BucketStoreConfig, shardingStrategy, storageBucket, limits, logger, prometheus.WrapRegistererWith(prometheus.Labels{"component": "store-gateway"}, reg))
	if err != nil {
		return nil, errors.Wrap(err, "create bucket stores")
	}

	g.Service = services.NewBasicService(g.starting, g.running, g.stopping)

	return g, nil
}

func (g *StoreGateway) starting(ctx context.Context) (err error) {
	// In case this function will return error we want to unregister the instance
	// from the ring. We do it ensuring dependencies are gracefully stopped if they
	// were already started.
	defer func() {
		if err == nil || g.subservices == nil {
			return
		}

		if stopErr := services.StopManagerAndAwaitStopped(context.Background(), g.subservices); stopErr != nil {
			level.Error(g.logger).Log("msg", "failed to gracefully stop store-gateway dependencies", "err", stopErr)
		}
	}()

	// First of all we register the instance in the ring and wait
	// until the lifecycler successfully started.
	if g.subservices, err = services.NewManager(g.ringLifecycler, g.ring); err != nil {
		return errors.Wrap(err, "unable to start store-gateway dependencies")
	}

	g.subservicesWatcher = services.NewFailureWatcher()
	g.subservicesWatcher.WatchManager(g.subservices)

	if err = services.StartManagerAndAwaitHealthy(ctx, g.subservices); err != nil {
		return errors.Wrap(err, "unable to start store-gateway dependencies")
	}

	// Wait until the ring client detected this instance in the JOINING state to
	// make sure that when we'll run the initial sync we already know  the tokens
	// assigned to this instance.
	level.Info(g.logger).Log("msg", "waiting until store-gateway is JOINING in the ring")
	if err := ring.WaitInstanceState(ctx, g.ring, g.ringLifecycler.GetInstanceID(), ring.JOINING); err != nil {
		return err
	}
	level.Info(g.logger).Log("msg", "store-gateway is JOINING in the ring")

	// In the event of a cluster cold start or scale up of 2+ store-gateway instances at the same
	// time, we may end up in a situation where each new store-gateway instance starts at a slightly
	// different time and thus each one starts with a different state of the ring. It's better
	// to just wait a short time for ring stability.
	if g.gatewayCfg.ShardingRing.WaitStabilityMinDuration > 0 {
		minWaiting := g.gatewayCfg.ShardingRing.WaitStabilityMinDuration
		maxWaiting := g.gatewayCfg.ShardingRing.WaitStabilityMaxDuration

		level.Info(g.logger).Log("msg", "waiting until store-gateway ring topology is stable", "min_waiting", minWaiting.String(), "max_waiting", maxWaiting.String())
		if err := ring.WaitRingTokensStability(ctx, g.ring, storegateway.BlocksOwnerSync, minWaiting, maxWaiting); err != nil {
			level.Warn(g.logger).Log("msg", "store-gateway ring topology is not stable after the max waiting time, proceeding anyway")
		} else {
			level.Info(g.logger).Log("msg", "store-gateway ring topology is stable")
		}
	}

	// At this point, if sharding is enabled, the instance is registered with some tokens
	// and we can run the initial synchronization.
	g.bucketSync.WithLabelValues(syncReasonInitial).Inc()
	if err = g.stores.InitialSync(ctx); err != nil {
		return errors.Wrap(err, "initial blocks synchronization")
	}

	// Now that the initial sync is done, we should have loaded all blocks
	// assigned to our shard, so we can switch to ACTIVE and start serving
	// requests.
	if err = g.ringLifecycler.ChangeState(ctx, ring.ACTIVE); err != nil {
		return errors.Wrapf(err, "switch instance to %s in the ring", ring.ACTIVE)
	}

	// Wait until the ring client detected this instance in the ACTIVE state to
	// make sure that when we'll run the loop it won't be detected as a ring
	// topology change.
	level.Info(g.logger).Log("msg", "waiting until store-gateway is ACTIVE in the ring")
	if err := ring.WaitInstanceState(ctx, g.ring, g.ringLifecycler.GetInstanceID(), ring.ACTIVE); err != nil {
		return err
	}
	level.Info(g.logger).Log("msg", "store-gateway is ACTIVE in the ring")

	return nil
}

func (g *StoreGateway) running(ctx context.Context) error {
	// Apply a jitter to the sync frequency in order to increase the probability
	// of hitting the shared cache (if any).
	syncTicker := time.NewTicker(util.DurationWithJitter(g.gatewayCfg.BucketStoreConfig.SyncInterval, 0.2))
	defer syncTicker.Stop()

	ringLastState, _ := g.ring.GetAllHealthy(storegateway.BlocksOwnerSync) // nolint:errcheck
	ringTicker := time.NewTicker(util.DurationWithJitter(g.gatewayCfg.ShardingRing.RingCheckPeriod, 0.2))
	defer ringTicker.Stop()

	for {
		select {
		case <-syncTicker.C:
			g.syncStores(ctx, syncReasonPeriodic)
		case <-ringTicker.C:
			// We ignore the error because in case of error it will return an empty
			// replication set which we use to compare with the previous state.
			currRingState, _ := g.ring.GetAllHealthy(storegateway.BlocksOwnerSync) // nolint:errcheck

			if ring.HasReplicationSetChanged(ringLastState, currRingState) {
				ringLastState = currRingState
				g.syncStores(ctx, syncReasonRingChange)
			}
		case <-ctx.Done():
			return nil
		case err := <-g.subservicesWatcher.Chan():
			return errors.Wrap(err, "store gateway subservice failed")
		}
	}
}

func (g *StoreGateway) stopping(_ error) error {
	if g.subservices != nil {
		if err := services.StopManagerAndAwaitStopped(context.Background(), g.subservices); err != nil {
			level.Warn(g.logger).Log("msg", "failed to stop store-gateway subservices", "err", err)
		}
	}

	return nil
}

func (g *StoreGateway) syncStores(ctx context.Context, reason string) {
	level.Info(g.logger).Log("msg", "synchronizing TSDB blocks for all users", "reason", reason)
	g.bucketSync.WithLabelValues(reason).Inc()

	if err := g.stores.SyncBlocks(ctx); err != nil {
		level.Warn(g.logger).Log("msg", "failed to synchronize TSDB blocks", "reason", reason, "err", err)
	} else {
		level.Info(g.logger).Log("msg", "successfully synchronized TSDB blocks for all users", "reason", reason)
	}
}
