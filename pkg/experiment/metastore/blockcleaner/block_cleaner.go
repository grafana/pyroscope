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
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
)

const (
	removedBlocksBucketName = "removed-blocks"
)

var removedBlocksBucketNameBytes = []byte(removedBlocksBucketName)

type Config struct {
	CompactedBlocksCleanupDelay model.Duration `yaml:"compacted_blocks_cleanup_delay"`
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	_ = cfg.CompactedBlocksCleanupDelay.Set("1m")
	f.Var(&cfg.CompactedBlocksCleanupDelay, prefix+"compacted-blocks-cleanup-delay", "")
}

type CleanerLifecycler interface {
	Cleaner

	Start()
	Stop()
}

type Cleaner interface {
	MarkBlock(shard uint32, tenant string, blockId string, deletedTs int64) error
	IsMarked(blockId string) bool

	RemoveExpiredBlocks(now int64) error
}

type RaftLog[Req, Resp proto.Message] interface {
	ApplyCommand(req Req) (resp Resp, err error)
}

type RaftLogCleanBlocks RaftLog[*raftlogpb.CleanBlocksRequest, *anypb.Any]

type blockCleaner struct {
	blocks   map[string]struct{}
	blocksMu sync.Mutex

	raftLog RaftLogCleanBlocks
	db      func() *bbolt.DB
	bkt     objstore.Bucket
	logger  log.Logger
	cfg     *Config

	started  bool
	mu       sync.Mutex
	wg       sync.WaitGroup
	cancel   func()
	isLeader bool
}

func New(raftLog RaftLogCleanBlocks, db func() *bbolt.DB, logger log.Logger, config *Config, bkt objstore.Bucket) CleanerLifecycler {
	return newBlockCleaner(raftLog, db, logger, config, bkt)
}

func newBlockCleaner(raftLog RaftLogCleanBlocks, db func() *bbolt.DB, logger log.Logger, config *Config, bkt objstore.Bucket) *blockCleaner {
	return &blockCleaner{
		blocks:  make(map[string]struct{}),
		raftLog: raftLog,
		db:      db,
		logger:  logger,
		cfg:     config,
		bkt:     bkt,
	}
}

type blockRemovalContext struct {
	tenant   string
	expiryTs int64
}

func (c *blockCleaner) MarkBlock(shard uint32, tenant string, blockId string, deletedTs int64) error {
	if c.IsMarked(blockId) {
		return nil
	}
	err := c.db().Update(func(tx *bbolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(removedBlocksBucketNameBytes)
		if err != nil {
			return err
		}
		shardBkt, err := getOrCreateSubBucket(bkt, getShardBucketName(shard))
		if err != nil {
			return err
		}
		expiryTs := deletedTs + time.Duration(c.cfg.CompactedBlocksCleanupDelay).Milliseconds()
		blockKey := getBlockKey(blockId, expiryTs, tenant)

		return shardBkt.Put(blockKey, []byte{})
	})
	if err != nil {
		return err
	}
	c.blocksMu.Lock()
	defer c.blocksMu.Unlock()
	c.blocks[blockId] = struct{}{}
	return nil
}

func (c *blockCleaner) IsMarked(blockId string) bool {
	c.blocksMu.Lock()
	defer c.blocksMu.Unlock()
	_, ok := c.blocks[blockId]
	return ok
}

func (c *blockCleaner) Start() {
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

func (c *blockCleaner) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		level.Warn(c.logger).Log("msg", "block cleaner already stopped")
		return
	}
	c.cancel()
	c.started = false
	c.wg.Wait()
	level.Info(c.logger).Log("msg", "block cleaner stopped")
}

func (c *blockCleaner) loop(ctx context.Context) {
	t := time.NewTicker(1 * time.Minute) // TODO(aleks-p): Make configurable
	defer func() {
		t.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if c.isLeader {
				_, err := c.raftLog.ApplyCommand(&raftlogpb.CleanBlocksRequest{})
				if err != nil {
					_ = level.Error(c.logger).Log("msg", "failed to apply truncate command", "err", err)
				}
			}
		}
	}
}

func (c *blockCleaner) RemoveExpiredBlocks(now int64) error {
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

func (c *blockCleaner) listShards() ([]uint32, error) {
	shards := make([]uint32, 0)
	err := c.db().View(func(tx *bbolt.Tx) error {
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

func (c *blockCleaner) cleanShard(ctx context.Context, shard uint32, now int64) error {
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
			// TODO(aleks-p): add more logging, metrics
			cntDeleted++
		}
	}
	level.Info(c.logger).Log("msg", "finished shard cleanup", "shard", shard, "blocks_removed", cntDeleted, "blocks_removed_bucket", cntDeletedBucket)
	return nil
}

func (c *blockCleaner) listBlocks(shard uint32) (map[string]*blockRemovalContext, error) {
	blocks := make(map[string]*blockRemovalContext)
	err := c.db().View(func(tx *bbolt.Tx) error {
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

func (c *blockCleaner) removeBlock(blockId string, shard uint32, removalContext *blockRemovalContext) error {
	err := c.db().Update(func(tx *bbolt.Tx) error {
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
