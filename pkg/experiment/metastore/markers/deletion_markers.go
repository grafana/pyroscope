package markers

import (
	"encoding/binary"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/util"
)

type metrics struct {
	markedBlocks  *prometheus.CounterVec
	expiredBlocks *prometheus.CounterVec
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
	}
	if reg != nil {
		util.Register(reg,
			m.markedBlocks,
			m.expiredBlocks,
		)
	}
	return m
}

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

type BlockRemovalContext struct {
	Shard    uint32
	Tenant   string
	ExpiryTs int64
}

type DeletionMarkers struct {
	blockMarkers map[string]*BlockRemovalContext
	mu           sync.Mutex

	logger  log.Logger
	cfg     *Config
	metrics *metrics
}

func NewDeletionMarkers(logger log.Logger, cfg *Config, reg prometheus.Registerer) *DeletionMarkers {
	return &DeletionMarkers{
		blockMarkers: make(map[string]*BlockRemovalContext),
		logger:       logger,
		cfg:          cfg,
		metrics:      newMetrics(reg),
	}
}

func (m *DeletionMarkers) Restore(tx *bbolt.Tx) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.blockMarkers)
	bkt, err := tx.CreateBucketIfNotExists(removedBlocksBucketNameBytes)
	if err != nil {
		return err
	}
	err = bkt.ForEachBucket(func(k []byte) error {
		shardBkt := bkt.Bucket(k)
		if shardBkt == nil {
			return nil
		}
		shard := binary.BigEndian.Uint32(k)
		return shardBkt.ForEach(func(k, v []byte) error {
			if len(k) < 34 {
				return fmt.Errorf("block key too short (expected 34 chars, was %d)", len(k))
			}
			blockId := string(k[:26])
			m.blockMarkers[blockId] = &BlockRemovalContext{
				Shard:    shard,
				Tenant:   string(k[34:]),
				ExpiryTs: int64(binary.BigEndian.Uint64(k[26:34])),
			}
			return nil
		})
	})
	if err != nil {
		return err
	}

	level.Info(m.logger).Log("msg", "loaded metastore block deletion markers", "marker_count", len(m.blockMarkers))
	return nil
}

func (m *DeletionMarkers) Mark(tx *bbolt.Tx, shard uint32, tenant string, blockId string, deletedTs int64) error {
	if m.IsMarked(blockId) {
		return nil
	}
	expiryTs := deletedTs + m.cfg.CompactedBlocksCleanupDelay.Milliseconds()
	bkt, err := tx.CreateBucketIfNotExists(removedBlocksBucketNameBytes)
	if err != nil {
		return err
	}
	shardBkt, err := getOrCreateSubBucket(bkt, getShardBucketName(shard))
	if err != nil {
		return err
	}
	blockKey := getBlockKey(blockId, expiryTs, tenant)
	if err = shardBkt.Put(blockKey, []byte{}); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockMarkers[blockId] = &BlockRemovalContext{
		Shard:    shard,
		Tenant:   tenant,
		ExpiryTs: expiryTs,
	}
	m.metrics.markedBlocks.WithLabelValues(tenant, fmt.Sprint(shard)).Inc()
	return nil
}

func (m *DeletionMarkers) IsMarked(blockId string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.blockMarkers[blockId]
	return ok
}

func (m *DeletionMarkers) FindExpiredMarkers(now int64) map[string]*BlockRemovalContext {
	blocks := make(map[string]*BlockRemovalContext)
	m.mu.Lock()
	defer m.mu.Unlock()
	for b, removalContext := range m.blockMarkers {
		if removalContext.ExpiryTs < now {
			blocks[b] = removalContext
		}
	}
	return blocks
}

func (m *DeletionMarkers) Remove(tx *bbolt.Tx, markers map[string]*BlockRemovalContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(markers) == 0 {
		return nil
	}
	markersPerShard := make(map[uint32]map[string]*BlockRemovalContext)
	for blockId, removalContext := range markers {
		s, ok := markersPerShard[removalContext.Shard]
		if !ok {
			s = make(map[string]*BlockRemovalContext)
			markersPerShard[removalContext.Shard] = s
		}
		s[blockId] = removalContext
	}
	bkt, err := getPendingBlockRemovalsBucket(tx)
	if err != nil {
		return err
	}
	for shard, shardMarkers := range markersPerShard {
		shardBkt, err := getOrCreateSubBucket(bkt, getShardBucketName(shard))
		if err != nil {
			return err
		}
		for b, m := range shardMarkers {
			key := getBlockKey(b, m.ExpiryTs, m.Tenant)
			err := shardBkt.Delete(key)
			if err != nil {
				return err
			}
		}
	}
	if err != nil {
		return err
	}
	for b, removalContext := range markers {
		delete(m.blockMarkers, b)
		level.Debug(m.logger).Log(
			"msg", "removed block from pending block removals",
			"blockId", b,
			"Shard", removalContext.Shard,
			"Tenant", removalContext.Tenant,
			"ExpiryTs", removalContext.ExpiryTs)
		m.metrics.expiredBlocks.WithLabelValues(removalContext.Tenant, fmt.Sprint(removalContext.Shard)).Inc()
	}
	level.Info(m.logger).Log("msg", "finished deletion marker cleanup", "markers_removed", len(markers))
	return nil
}

func getPendingBlockRemovalsBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	return tx.CreateBucketIfNotExists(removedBlocksBucketNameBytes)
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
