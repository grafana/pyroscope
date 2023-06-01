package storegateway

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/dskit/multierror"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	phlareobj "github.com/grafana/phlare/pkg/objstore"
	"github.com/grafana/phlare/pkg/phlaredb/bucket"
	"github.com/grafana/phlare/pkg/util"
)

var errBucketStoreNotFound = errors.New("bucket store not found")

type BucketStoreConfig struct {
	SyncDir               string        `yaml:"sync_dir"`
	SyncInterval          time.Duration `yaml:"sync_interval" category:"advanced"`
	TenantSyncConcurrency int           `yaml:"tenant_sync_concurrency" category:"advanced"`
	IgnoreBlocksWithin    time.Duration `yaml:"ignore_blocks_within" category:"advanced"`
}

// RegisterFlags registers the BucketStore flags
func (cfg *BucketStoreConfig) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	// cfg.IndexCache.RegisterFlagsWithPrefix(f, "blocks-storage.bucket-store.index-cache.")
	// cfg.ChunksCache.RegisterFlagsWithPrefix(f, "blocks-storage.bucket-store.chunks-cache.", logger)
	// cfg.MetadataCache.RegisterFlagsWithPrefix(f, "blocks-storage.bucket-store.metadata-cache.")
	// cfg.BucketIndex.RegisterFlagsWithPrefix(f, "blocks-storage.bucket-store.bucket-index.")
	// cfg.IndexHeader.RegisterFlagsWithPrefix(f, "blocks-storage.bucket-store.index-header.")

	f.StringVar(&cfg.SyncDir, "blocks-storage.bucket-store.sync-dir", "./data/pyroscope-sync/", "Directory to store synchronized pyroscope block headers. This directory is not required to be persisted between restarts, but it's highly recommended in order to improve the store-gateway startup time.")
	f.DurationVar(&cfg.SyncInterval, "blocks-storage.bucket-store.sync-interval", 15*time.Minute, "How frequently to scan the bucket, or to refresh the bucket index (if enabled), in order to look for changes (new blocks shipped by ingesters and blocks deleted by retention or compaction).")
	f.IntVar(&cfg.TenantSyncConcurrency, "blocks-storage.bucket-store.tenant-sync-concurrency", 10, "Maximum number of concurrent tenants synching blocks.")
	f.DurationVar(&cfg.IgnoreBlocksWithin, "blocks-storage.bucket-store.ignore-blocks-within", 2*time.Hour, "Blocks with minimum time within this duration are ignored, and not loaded by store-gateway. Useful when used together with -querier.query-store-after to prevent loading young blocks, because there are usually many of them (depending on number of ingesters) and they are not yet compacted. Negative values or 0 disable the filter.")

	// f.Uint64Var(&cfg.MaxChunkPoolBytes, "blocks-storage.bucket-store.max-chunk-pool-bytes", uint64(2*units.Gibibyte), "Max size - in bytes - of a chunks pool, used to reduce memory allocations. The pool is shared across all tenants. 0 to disable the limit.")
	// f.IntVar(&cfg.ChunkPoolMinBucketSizeBytes, "blocks-storage.bucket-store.chunk-pool-min-bucket-size-bytes", ChunkPoolDefaultMinBucketSize, "Size - in bytes - of the smallest chunks pool bucket.")
	// f.IntVar(&cfg.ChunkPoolMaxBucketSizeBytes, "blocks-storage.bucket-store.chunk-pool-max-bucket-size-bytes", ChunkPoolDefaultMaxBucketSize, "Size - in bytes - of the largest chunks pool bucket.")
	// f.Uint64Var(&cfg.SeriesHashCacheMaxBytes, "blocks-storage.bucket-store.series-hash-cache-max-size-bytes", uint64(1*units.Gibibyte), "Max size - in bytes - of the in-memory series hash cache. The cache is shared across all tenants and it's used only when query sharding is enabled.")
	// f.IntVar(&cfg.MaxConcurrent, "blocks-storage.bucket-store.max-concurrent", 100, "Max number of concurrent queries to execute against the long-term storage. The limit is shared across all tenants.")
	// f.IntVar(&cfg.BlockSyncConcurrency, "blocks-storage.bucket-store.block-sync-concurrency", 20, "Maximum number of concurrent blocks synching per tenant.")
	// f.IntVar(&cfg.MetaSyncConcurrency, "blocks-storage.bucket-store.meta-sync-concurrency", 20, "Number of Go routines to use when syncing block meta files from object storage per tenant.")
	// f.DurationVar(&cfg.DeprecatedConsistencyDelay, consistencyDelayFlag, 0, "Minimum age of a block before it's being read. Set it to safe value (e.g 30m) if your object storage is eventually consistent. GCS and S3 are (roughly) strongly consistent.")
	// f.DurationVar(&cfg.IgnoreDeletionMarksDelay, "blocks-storage.bucket-store.ignore-deletion-marks-delay", time.Hour*1, "Duration after which the blocks marked for deletion will be filtered out while fetching blocks. "+
	// 	"The idea of ignore-deletion-marks-delay is to ignore blocks that are marked for deletion with some delay. This ensures store can still serve blocks that are meant to be deleted but do not have a replacement yet.")
	// f.IntVar(&cfg.PostingOffsetsInMemSampling, "blocks-storage.bucket-store.posting-offsets-in-mem-sampling", DefaultPostingOffsetInMemorySampling, "Controls what is the ratio of postings offsets that the store will hold in memory.")
	// f.BoolVar(&cfg.IndexHeaderLazyLoadingEnabled, "blocks-storage.bucket-store.index-header-lazy-loading-enabled", true, "If enabled, store-gateway will lazy load an index-header only once required by a query.")
	// f.DurationVar(&cfg.IndexHeaderLazyLoadingIdleTimeout, "blocks-storage.bucket-store.index-header-lazy-loading-idle-timeout", 60*time.Minute, "If index-header lazy loading is enabled and this setting is > 0, the store-gateway will offload unused index-headers after 'idle timeout' inactivity.")
	// f.Uint64Var(&cfg.PartitionerMaxGapBytes, "blocks-storage.bucket-store.partitioner-max-gap-bytes", DefaultPartitionerMaxGapSize, "Max size - in bytes - of a gap for which the partitioner aggregates together two bucket GET object requests.")
	// f.IntVar(&cfg.StreamingBatchSize, "blocks-storage.bucket-store.batch-series-size", 5000, "This option controls how many series to fetch per batch. The batch size must be greater than 0.")
	// f.IntVar(&cfg.ChunkRangesPerSeries, "blocks-storage.bucket-store.fine-grained-chunks-caching-ranges-per-series", 1, "This option controls into how many ranges the chunks of each series from each block are split. This value is effectively the number of chunks cache items per series per block when -blocks-storage.bucket-store.chunks-cache.fine-grained-chunks-caching-enabled is enabled.")
	// f.StringVar(&cfg.SeriesSelectionStrategyName, "blocks-storage.bucket-store.series-selection-strategy", AllPostingsStrategy, "This option controls the strategy to selection of series and deferring application of matchers. A more aggressive strategy will fetch less posting lists at the cost of more series. This is useful when querying large blocks in which many series share the same label name and value. Supported values (most aggressive to least aggressive): "+strings.Join(validSeriesSelectionStrategies, ", ")+".")
}

// Validate the config.
func (cfg *BucketStoreConfig) Validate(logger log.Logger) error {
	// if cfg.StreamingBatchSize <= 0 {
	// 	return errInvalidStreamingBatchSize
	// }
	// if err := cfg.IndexCache.Validate(); err != nil {
	// 	return errors.Wrap(err, "index-cache configuration")
	// }
	// if err := cfg.ChunksCache.Validate(); err != nil {
	// 	return errors.Wrap(err, "chunks-cache configuration")
	// }
	// if err := cfg.MetadataCache.Validate(); err != nil {
	// 	return errors.Wrap(err, "metadata-cache configuration")
	// }
	// if cfg.DeprecatedConsistencyDelay > 0 {
	// 	util.WarnDeprecatedConfig(consistencyDelayFlag, logger)
	// }
	// if !util.StringsContain(validSeriesSelectionStrategies, cfg.SeriesSelectionStrategyName) {
	// 	return errors.New("invalid series-selection-strategy, set one of " + strings.Join(validSeriesSelectionStrategies, ", "))
	// }
	return nil
}

type BucketStores struct {
	storageBucket     phlareobj.Bucket
	cfg               BucketStoreConfig
	logger            log.Logger
	syncBackoffConfig backoff.Config
	shardingStrategy  ShardingStrategy
	limits            Limits
	reg               prometheus.Registerer
	// Keeps a bucket store for each tenant.
	storesMu sync.RWMutex
	stores   map[string]*BucketStore

	// Metrics.
	syncTimes         prometheus.Histogram
	syncLastSuccess   prometheus.Gauge
	tenantsDiscovered prometheus.Gauge
	tenantsSynced     prometheus.Gauge
	blocksLoaded      prometheus.GaugeFunc
	metrics           *Metrics
}

func NewBucketStores(cfg BucketStoreConfig, shardingStrategy ShardingStrategy, storageBucket phlareobj.Bucket, limits Limits, logger log.Logger, reg prometheus.Registerer) (*BucketStores, error) {
	bs := &BucketStores{
		storageBucket: storageBucket,
		logger:        logger,
		cfg:           cfg,
		syncBackoffConfig: backoff.Config{
			MinBackoff: 1 * time.Second,
			MaxBackoff: 10 * time.Second,
			MaxRetries: 3,
		},
		stores:           map[string]*BucketStore{},
		shardingStrategy: shardingStrategy,
		reg:              reg,
		limits:           limits,
		metrics:          NewMetrics(reg),
	}
	// Register metrics.
	bs.syncTimes = promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
		Name:    "pyroscope_bucket_stores_blocks_sync_seconds",
		Help:    "The total time it takes to perform a sync stores",
		Buckets: []float64{0.1, 1, 10, 30, 60, 120, 300, 600, 900},
	})
	bs.syncLastSuccess = promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Name: "pyroscope_bucket_stores_blocks_last_successful_sync_timestamp_seconds",
		Help: "Unix timestamp of the last successful blocks sync.",
	})
	bs.tenantsDiscovered = promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Name: "pyroscope_bucket_stores_tenants_discovered",
		Help: "Number of tenants discovered in the bucket.",
	})
	bs.tenantsSynced = promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Name: "pyroscope_bucket_stores_tenants_synced",
		Help: "Number of tenants synced.",
	})
	bs.blocksLoaded = promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "pyroscope_bucket_store_blocks_loaded",
		Help: "Number of currently loaded blocks.",
	}, bs.getBlocksLoadedMetric)
	return bs, nil
}

// SyncBlocks synchronizes the stores state with the Bucket store for every user.
func (bs *BucketStores) SyncBlocks(ctx context.Context) error {
	return bs.syncUsersBlocksWithRetries(ctx, func(ctx context.Context, s *BucketStore) error {
		return s.SyncBlocks(ctx)
	})
}

func (bs *BucketStores) InitialSync(ctx context.Context) error {
	level.Info(bs.logger).Log("msg", "synchronizing Pyroscope blocks for all users")

	if err := bs.syncUsersBlocksWithRetries(ctx, func(ctx context.Context, s *BucketStore) error {
		return s.InitialSync(ctx)
	}); err != nil {
		level.Warn(bs.logger).Log("msg", "failed to synchronize Pyroscope blocks", "err", err)
		return err
	}

	level.Info(bs.logger).Log("msg", "successfully synchronized Pyroscope blocks for all users")
	return nil
}

func (bs *BucketStores) syncUsersBlocksWithRetries(ctx context.Context, f func(context.Context, *BucketStore) error) error {
	retries := backoff.New(ctx, bs.syncBackoffConfig)

	var lastErr error
	for retries.Ongoing() {
		lastErr = bs.syncUsersBlocks(ctx, f)
		if lastErr == nil {
			return nil
		}

		retries.Wait()
	}

	if lastErr == nil {
		return retries.Err()
	}

	return lastErr
}

func (bs *BucketStores) syncUsersBlocks(ctx context.Context, f func(context.Context, *BucketStore) error) (returnErr error) {
	defer func(start time.Time) {
		bs.syncTimes.Observe(time.Since(start).Seconds())
		if returnErr == nil {
			bs.syncLastSuccess.SetToCurrentTime()
		}
	}(time.Now())

	type job struct {
		userID string
		store  *BucketStore
	}

	wg := &sync.WaitGroup{}
	jobs := make(chan job)
	errs := multierror.New()
	errsMx := sync.Mutex{}

	// Scan users in the bucket. In case of error, it may return a subset of users. If we sync a subset of users
	// during a periodic sync, we may end up unloading blocks for users that still belong to this store-gateway
	// so we do prefer to not run the sync at all.
	userIDs, err := bs.scanUsers(ctx)
	if err != nil {
		return err
	}

	ownedUserIDs, err := bs.shardingStrategy.FilterUsers(ctx, userIDs)
	if err != nil {
		return errors.Wrap(err, "unable to check tenants owned by this store-gateway instance")
	}

	includeUserIDs := make(map[string]struct{}, len(ownedUserIDs))
	for _, userID := range ownedUserIDs {
		includeUserIDs[userID] = struct{}{}
	}

	bs.tenantsDiscovered.Set(float64(len(userIDs)))
	bs.tenantsSynced.Set(float64(len(includeUserIDs)))

	// Create a pool of workers which will synchronize blocks. The pool size
	// is limited in order to avoid to concurrently sync a lot of tenants in
	// a large cluster.
	for i := 0; i < bs.cfg.TenantSyncConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for job := range jobs {
				if err := f(ctx, job.store); err != nil {
					errsMx.Lock()
					errs.Add(errors.Wrapf(err, "failed to synchronize Pyroscope blocks for user %s", job.userID))
					errsMx.Unlock()
				}
			}
		}()
	}

	// Lazily create a bucket store for each new user found
	// and submit a sync job for each user.
	for userID := range includeUserIDs {
		bs, err := bs.getOrCreateStore(userID)
		if err != nil {
			errsMx.Lock()
			errs.Add(err)
			errsMx.Unlock()

			continue
		}

		select {
		case jobs <- job{userID: userID, store: bs}:
			// Nothing to do. Will loop to push more jobs.
		case <-ctx.Done():
			// Wait until all workers have done, so the goroutines leak detector doesn't
			// report any issue. This is expected to be quick, considering the done ctx
			// is used by the worker callback function too.
			close(jobs)
			wg.Wait()

			return ctx.Err()
		}
	}

	// Wait until all workers completed.
	close(jobs)
	wg.Wait()

	bs.closeBucketStoreAndDeleteLocalFilesForExcludedTenants(includeUserIDs)

	return errs.Err()
}

func (bs *BucketStores) getStore(userID string) *BucketStore {
	bs.storesMu.RLock()
	defer bs.storesMu.RUnlock()
	return bs.stores[userID]
}

func (bs *BucketStores) getOrCreateStore(userID string) (*BucketStore, error) {
	// Check if the store already exists.
	s := bs.getStore(userID)
	if s != nil {
		return s, nil
	}

	bs.storesMu.Lock()
	defer bs.storesMu.Unlock()

	// Check again for the store in the event it was created in-between locks.
	s = bs.stores[userID]
	if s != nil {
		return s, nil
	}

	userLogger := util.LoggerWithUserID(userID, bs.logger)

	level.Info(userLogger).Log("msg", "creating user bucket store")

	// The sharding strategy filter MUST be before the ones we create here (order matters).
	filters := []BlockMetaFilter{
		NewShardingMetadataFilterAdapter(userID, bs.shardingStrategy),
		// block.NewConsistencyDelayMetaFilter(userLogger, u.cfg.BucketStore.DeprecatedConsistencyDelay, fetcherReg),
		newMinTimeMetaFilter(bs.cfg.IgnoreBlocksWithin),
	}

	s, err := NewBucketStore(
		bs.storageBucket,
		userID,
		bs.syncDirForUser(userID),
		filters,
		userLogger,
		bs.metrics,
	)
	if err != nil {
		return nil, err
	}

	bs.stores[userID] = s

	return s, nil
}

// closeBucketStoreAndDeleteLocalFilesForExcludedTenants closes bucket store and removes local "sync" directories
// for tenants that are not included in the current shard.
func (bs *BucketStores) closeBucketStoreAndDeleteLocalFilesForExcludedTenants(includeUserIDs map[string]struct{}) {
	files, err := os.ReadDir(bs.cfg.SyncDir)
	if err != nil {
		return
	}

	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		userID := f.Name()
		if _, included := includeUserIDs[userID]; included {
			// Preserve directory for users owned by this shard.
			continue
		}

		err := bs.closeBucketStore(userID)
		switch {
		case errors.Is(err, errBucketStoreNotFound):
			// This is OK, nothing was closed.
		case err == nil:
			level.Info(bs.logger).Log("msg", "closed bucket store for user", "user", userID)
		default:
			level.Warn(bs.logger).Log("msg", "failed to close bucket store for user", "user", userID, "err", err)
		}

		userSyncDir := bs.syncDirForUser(userID)
		err = os.RemoveAll(userSyncDir)
		if err == nil {
			level.Info(bs.logger).Log("msg", "deleted user sync directory", "dir", userSyncDir)
		} else {
			level.Warn(bs.logger).Log("msg", "failed to delete user sync directory", "dir", userSyncDir, "err", err)
		}
	}
}

func (u *BucketStores) syncDirForUser(userID string) string {
	return filepath.Join(u.cfg.SyncDir, userID)
}

// closeBucketStore closes bucket store for given user
// and removes it from bucket stores map and metrics.
// If bucket store doesn't exist, returns errBucketStoreNotFound.
// Otherwise returns error from closing the bucket store.
func (bs *BucketStores) closeBucketStore(userID string) error {
	bs.storesMu.Lock()
	unlockInDefer := true
	defer func() {
		if unlockInDefer {
			bs.storesMu.Unlock()
		}
	}()

	s := bs.stores[userID]
	if bs == nil {
		return errBucketStoreNotFound
	}

	delete(bs.stores, userID)
	unlockInDefer = false
	bs.storesMu.Unlock()

	return s.RemoveBlocksAndClose()
}

// getBlocksLoadedMetric returns the number of blocks currently loaded across all bucket stores.
func (u *BucketStores) getBlocksLoadedMetric() float64 {
	count := 0

	u.storesMu.RLock()
	for _, store := range u.stores {
		count += store.Stats().BlocksLoaded
	}
	u.storesMu.RUnlock()

	return float64(count)
}

func (bs *BucketStores) scanUsers(ctx context.Context) ([]string, error) {
	return bucket.ListUsers(ctx, bs.storageBucket)
}
