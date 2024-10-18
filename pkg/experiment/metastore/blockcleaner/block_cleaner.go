package blockcleaner

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftleader"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
	"github.com/grafana/pyroscope/pkg/util"
)

const (
	removedBlocksBucketName = "removed-blocks"
)

var removedBlocksBucketNameBytes = []byte(removedBlocksBucketName)

type Config struct {
	CompactedBlocksCleanupInterval time.Duration `yaml:"compacted_blocks_cleanup_interval"`
	CompactedBlocksCleanupDelay    time.Duration `yaml:"compacted_blocks_cleanup_delay"`
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.CompactedBlocksCleanupDelay, prefix+"compacted-blocks-cleanup-delay", time.Minute*30, "The grace period for permanently deleting compacted blocks.")
	f.DurationVar(&cfg.CompactedBlocksCleanupInterval, prefix+"compacted-blocks-cleanup-interval", time.Minute, "The interval at which block cleanup is performed.")
}

type CleanerLifecycler interface {
	raftleader.LeaderRoutine
	LoadMarkers()
}

type RaftLog[Req, Resp proto.Message] interface {
	ApplyCommand(req Req) (resp Resp, err error)
}

type RaftLogCleanBlocks RaftLog[*raftlogpb.CleanBlocksRequest, *anypb.Any]

type metrics struct {
	markedBlocks         *prometheus.CounterVec
	expiredBlocks        *prometheus.CounterVec
	bucketObjectRemovals *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		markedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "metastore",
			Name:      "block_cleaner_marked_block_count",
			Help:      "The number of blocks marked as removed",
		}, []string{"tenant", "shard"}),
		expiredBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "metastore",
			Name:      "block_cleaner_expired_block_count",
			Help:      "The number of marked blocks that expired and were removed",
		}, []string{"tenant", "shard"}),
		bucketObjectRemovals: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "metastore",
			Name:      "block_cleaner_bucket_removal_count",
			Help:      "The number of expired blocks that were removed from the bucket",
		}, []string{"tenant", "shard"}),
	}
	if reg != nil {
		util.Register(reg,
			m.markedBlocks,
			m.expiredBlocks,
			m.bucketObjectRemovals,
		)
	}
	return m
}

type BlockCleaner struct {
	blocks   map[string]struct{}
	blocksMu sync.Mutex

	raftLog RaftLogCleanBlocks
	db      *bbolt.DB
	bkt     objstore.Bucket
	logger  log.Logger
	cfg     *Config
	metrics *metrics

	started  bool
	mu       sync.Mutex
	wg       sync.WaitGroup
	cancel   func()
	isLeader bool
}

func New(
	raftLog RaftLogCleanBlocks,
	db *bbolt.DB,
	logger log.Logger,
	config *Config,
	bkt objstore.Bucket,
	reg prometheus.Registerer,
) *BlockCleaner {
	return newBlockCleaner(raftLog, db, logger, config, bkt, reg)
}

func newBlockCleaner(
	raftLog RaftLogCleanBlocks,
	db *bbolt.DB,
	logger log.Logger,
	config *Config,
	bkt objstore.Bucket,
	reg prometheus.Registerer,
) *BlockCleaner {
	return &BlockCleaner{
		blocks:  make(map[string]struct{}),
		raftLog: raftLog,
		db:      db,
		logger:  logger,
		cfg:     config,
		bkt:     bkt,
		metrics: newMetrics(reg),
	}
}

type blockRemovalContext struct {
	tenant   string
	expiryTs int64
}

func (c *BlockCleaner) LoadMarkers() {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket(removedBlocksBucketNameBytes)
		if bkt == nil {
			return nil
		}
		return bkt.ForEachBucket(func(k []byte) error {
			shardBkt := bkt.Bucket(k)
			if shardBkt == nil {
				return nil
			}
			return shardBkt.ForEach(func(k, v []byte) error {
				if len(k) < 34 {
					return fmt.Errorf("block key too short (expected 34 chars, was %d)", len(k))
				}
				blockId := string(k[:26])
				c.blocks[blockId] = struct{}{}
				return nil
			})
		})
	})
	level.Info(c.logger).Log("msg", "loaded metastore block deletion markers", "marker_count", len(c.blocks))
}

func (c *BlockCleaner) MarkBlock(shard uint32, tenant string, blockId string, deletedTs int64) error {
	if c.IsMarked(blockId) {
		return nil
	}
	err := c.db.Update(func(tx *bbolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(removedBlocksBucketNameBytes)
		if err != nil {
			return err
		}
		shardBkt, err := getOrCreateSubBucket(bkt, getShardBucketName(shard))
		if err != nil {
			return err
		}
		expiryTs := deletedTs + c.cfg.CompactedBlocksCleanupDelay.Milliseconds()
		blockKey := getBlockKey(blockId, expiryTs, tenant)

		return shardBkt.Put(blockKey, []byte{})
	})
	if err != nil {
		return err
	}
	c.blocksMu.Lock()
	defer c.blocksMu.Unlock()
	c.blocks[blockId] = struct{}{}
	c.metrics.markedBlocks.WithLabelValues(tenant, fmt.Sprint(shard)).Inc()
	return nil
}

func (c *BlockCleaner) IsMarked(blockId string) bool {
	c.blocksMu.Lock()
	defer c.blocksMu.Unlock()
	_, ok := c.blocks[blockId]
	return ok
}

func (c *BlockCleaner) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		level.Info(c.logger).Log("msg", "blockc cleaner already started")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.started = true
	c.isLeader = true
	go c.loop(ctx)
	level.Info(c.logger).Log("msg", "block cleaner started")
}

func (c *BlockCleaner) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		level.Warn(c.logger).Log("msg", "block cleaner already stopped")
		return
	}
	c.cancel()
	c.started = false
	c.isLeader = false
	c.wg.Wait()
	level.Info(c.logger).Log("msg", "block cleaner stopped")
}

func (c *BlockCleaner) loop(ctx context.Context) {
	t := time.NewTicker(c.cfg.CompactedBlocksCleanupInterval)
	defer func() {
		t.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, err := c.raftLog.ApplyCommand(&raftlogpb.CleanBlocksRequest{})
			if err != nil {
				_ = level.Error(c.logger).Log("msg", "failed to apply clean blocks command", "err", err)
			}
		}
	}
}

func (c *BlockCleaner) RemoveExpiredBlocks(now int64) error {
	shards, err := c.listShards()
	if err != nil {
		panic(fmt.Errorf("failed to list shards for pending block removals: %w", err))
	}
	g, ctx := errgroup.WithContext(context.Background())
	for _, shard := range shards {
		g.Go(func() error {
			c.wg.Add(1)
			defer c.wg.Done()
			return c.cleanShard(ctx, shard, now)
		})
	}
	err = g.Wait()
	if err != nil {
		level.Warn(c.logger).Log("msg", "error during pending block removal", "err", err)
	}
	return err
}

func (c *BlockCleaner) listShards() ([]uint32, error) {
	shards := make([]uint32, 0)
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt, err := getPendingBlockRemovalsBucket(tx)
		if err != nil {
			return err
		}
		return bkt.ForEachBucket(func(k []byte) error {
			shards = append(shards, binary.BigEndian.Uint32(k))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return shards, nil
}

func (c *BlockCleaner) cleanShard(ctx context.Context, shard uint32, now int64) error {
	blocks, err := c.listBlocks(shard)
	if err != nil {
		level.Warn(c.logger).Log("msg", "failed to list removed blocks for shard", "err", err, "shard", shard)
		return err
	}
	level.Info(c.logger).Log("msg", "cleaning removed blocks in shard", "shard", shard, "blocks", len(blocks))
	cntDeleted := 0
	cntDeletedBucket := 0
	for blockId, removalContext := range blocks {
		if removalContext.expiryTs < now {
			metricLabels := []string{removalContext.tenant, fmt.Sprint(shard)}
			if c.isLeader {
				var key string
				if removalContext.tenant != "" {
					key = filepath.Join("blocks", fmt.Sprint(shard), removalContext.tenant, blockId, "block.bin")
				} else {
					key = filepath.Join("segments", fmt.Sprint(shard), "anonymous", blockId, "block.bin")
				}
				level.Debug(c.logger).Log(
					"msg", "removing block from bucket",
					"shard", shard,
					"tenant", removalContext.tenant,
					"blockId", blockId,
					"expiryTs", removalContext.expiryTs,
					"bucket_key", key)
				err := c.bkt.Delete(ctx, key)
				if err != nil {
					level.Warn(c.logger).Log(
						"msg", "failed to remove block from bucket",
						"err", err,
						"blockId", blockId,
						"shard", shard,
						"tenant", removalContext.tenant)
					// TODO(aleks-p): Detect if the error is "object does not exist" or something else. Handle each case appropriately.
					continue
				}
				c.metrics.bucketObjectRemovals.WithLabelValues(metricLabels...).Inc()
				cntDeletedBucket++
			}
			err = c.removeBlock(blockId, shard, removalContext)
			if err != nil {
				level.Warn(c.logger).Log(
					"msg", "failed to remove block from pending block removals",
					"err", err,
					"blockId", blockId,
					"shard", shard,
					"tenant", removalContext.tenant,
					"expiry", removalContext.expiryTs)
			}
			level.Debug(c.logger).Log(
				"msg", "removed block from pending block removals",
				"blockId", blockId,
				"shard", shard,
				"tenant", removalContext.tenant,
				"expiryTs", removalContext.expiryTs)
			c.metrics.expiredBlocks.WithLabelValues(metricLabels...).Inc()
			cntDeleted++
		}
	}
	level.Info(c.logger).Log("msg", "finished shard cleanup", "shard", shard, "blocks_removed", cntDeleted, "blocks_removed_bucket", cntDeletedBucket)
	return nil
}

func (c *BlockCleaner) listBlocks(shard uint32) (map[string]*blockRemovalContext, error) {
	blocks := make(map[string]*blockRemovalContext)
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt, err := getPendingBlockRemovalsBucket(tx)
		if err != nil {
			return err
		}
		shardBkt := bkt.Bucket(getShardBucketName(shard))
		if shardBkt == nil {
			return nil
		}
		return shardBkt.ForEach(func(k, v []byte) error {
			if len(k) < 34 {
				return fmt.Errorf("block key too short (expected 34 chars, was %d)", len(k))
			}
			blockId := string(k[:26])
			blocks[blockId] = &blockRemovalContext{
				expiryTs: int64(binary.BigEndian.Uint64(k[26:34])),
				tenant:   string(k[34:]),
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func (c *BlockCleaner) removeBlock(blockId string, shard uint32, removalContext *blockRemovalContext) error {
	err := c.db.Update(func(tx *bbolt.Tx) error {
		bkt, err := getPendingBlockRemovalsBucket(tx)
		if err != nil {
			return err
		}
		shardBkt := bkt.Bucket(getShardBucketName(shard))
		if shardBkt == nil {
			return errors.New("no bucket found for shard when clearing pending block removal")
		}
		blockKey := getBlockKey(blockId, removalContext.expiryTs, removalContext.tenant)

		return shardBkt.Delete(blockKey)
	})
	if err != nil {
		return err
	}
	c.blocksMu.Lock()
	defer c.blocksMu.Unlock()
	delete(c.blocks, blockId)
	return nil
}

func getPendingBlockRemovalsBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	bkt := tx.Bucket(removedBlocksBucketNameBytes)
	if bkt == nil {
		return nil, bbolt.ErrBucketNotFound
	}
	return bkt, nil
}

func getOrCreateSubBucket(parent *bbolt.Bucket, name []byte) (*bbolt.Bucket, error) {
	bucket := parent.Bucket(name)
	if bucket == nil {
		return parent.CreateBucket(name)
	}
	return bucket, nil
}

func getShardBucketName(shard uint32) []byte {
	shardBucketName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBucketName, shard)
	return shardBucketName
}

func getBlockKey(blockId string, expiryTs int64, tenant string) []byte {
	blockKey := make([]byte, 26+8+len(tenant))
	copy(blockKey[:26], blockId)
	binary.BigEndian.PutUint64(blockKey[26:34], uint64(expiryTs))
	copy(blockKey[34:], tenant)
	return blockKey
}
