package metastore

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

const (
	compactionBucketJobBlockQueuePrefix = "compaction-job-block-queue"
)

type tenantShard struct {
	tenant string
	shard  uint32
}

type metastoreState struct {
	logger            log.Logger
	compactionMetrics *compactionMetrics

	shardsMutex sync.Mutex
	shards      map[uint32]*metastoreShard

	compactionMutex          sync.Mutex
	compactionJobBlockQueues map[tenantShard]*compactionJobBlockQueue
	compactionJobQueue       *jobQueue

	db *boltdb
}

type metastoreShard struct {
	segmentsMutex sync.Mutex
	segments      map[string]*metastorev1.BlockMeta
}

type compactionJobBlockQueue struct {
	mu            sync.Mutex
	blocksByLevel map[uint32][]string
}

func newMetastoreState(logger log.Logger, db *boltdb, reg prometheus.Registerer) *metastoreState {
	return &metastoreState{
		logger:                   logger,
		shards:                   make(map[uint32]*metastoreShard),
		db:                       db,
		compactionJobBlockQueues: make(map[tenantShard]*compactionJobBlockQueue),
		compactionJobQueue:       newJobQueue(jobLeaseDuration.Nanoseconds()),
		compactionMetrics:        newCompactionMetrics(reg),
	}
}

func (m *metastoreState) reset(db *boltdb) {
	m.shardsMutex.Lock()
	m.compactionMutex.Lock()
	clear(m.shards)
	clear(m.compactionJobBlockQueues)
	m.compactionJobQueue = newJobQueue(jobLeaseDuration.Nanoseconds())
	m.db = db
	m.shardsMutex.Unlock()
	m.compactionMutex.Unlock()
}

func (m *metastoreState) getOrCreateShard(shardID uint32) *metastoreShard {
	m.shardsMutex.Lock()
	defer m.shardsMutex.Unlock()
	if shard, ok := m.shards[shardID]; ok {
		return shard
	}
	shard := newMetastoreShard()
	m.shards[shardID] = shard
	return shard
}

func (m *metastoreState) restore(db *boltdb) error {
	m.reset(db)
	return db.boltdb.View(func(tx *bbolt.Tx) error {
		if err := m.restoreBlockMetadata(tx); err != nil {
			return fmt.Errorf("failed to restore metadata entries: %w", err)
		}
		return m.restoreCompactionPlan(tx)
	})
}

func (m *metastoreState) restoreBlockMetadata(tx *bbolt.Tx) error {
	mdb, err := getBlockMetadataBucket(tx)
	switch {
	case err == nil:
	case errors.Is(err, bbolt.ErrBucketNotFound):
		return nil
	default:
		return err
	}
	// List shards in the block_metadata bucket:
	// block_metadata/[{shard_id}<tenant_id>]/[block_id]
	// TODO(kolesnikovae): Load concurrently.
	return mdb.ForEachBucket(func(name []byte) error {
		shardID, _, ok := parseBucketName(name)
		if !ok {
			_ = level.Error(m.logger).Log("msg", "malformed bucket name", "name", string(name))
			return nil
		}
		shard := m.getOrCreateShard(shardID)
		return shard.loadSegments(mdb.Bucket(name))
	})
}

func (m *metastoreState) restoreCompactionPlan(tx *bbolt.Tx) error {
	cdb, err := getCompactionJobBucket(tx)
	switch {
	case err == nil:
	case errors.Is(err, bbolt.ErrBucketNotFound):
		return nil
	default:
		return err
	}
	return cdb.ForEachBucket(func(name []byte) error {
		shard, tenant, ok := parseBucketName(name)
		if !ok {
			_ = level.Error(m.logger).Log("msg", "malformed bucket name", "name", string(name))
			return nil
		}
		key := tenantShard{
			tenant: tenant,
			shard:  shard,
		}
		blockQueue := m.getOrCreateCompactionBlockQueue(key)

		return m.loadCompactionPlan(cdb.Bucket(name), blockQueue)
	})

}

func (m *metastoreState) getOrCreateCompactionBlockQueue(key tenantShard) *compactionJobBlockQueue {
	m.compactionMutex.Lock()
	defer m.compactionMutex.Unlock()

	if blockQueue, ok := m.compactionJobBlockQueues[key]; ok {
		return blockQueue
	}
	plan := &compactionJobBlockQueue{
		blocksByLevel: make(map[uint32][]string),
	}
	m.compactionJobBlockQueues[key] = plan
	return plan
}

func (m *metastoreState) findJob(name string) *compactionpb.CompactionJob {
	m.compactionJobQueue.mu.Lock()
	defer m.compactionJobQueue.mu.Unlock()
	if jobEntry, exists := m.compactionJobQueue.jobs[name]; exists {
		return jobEntry.CompactionJob
	}
	return nil
}

func newMetastoreShard() *metastoreShard {
	return &metastoreShard{
		segments: make(map[string]*metastorev1.BlockMeta),
	}
}

func (s *metastoreShard) putSegment(segment *metastorev1.BlockMeta) {
	s.segmentsMutex.Lock()
	s.segments[segment.Id] = segment
	s.segmentsMutex.Unlock()
}

func (s *metastoreShard) deleteSegment(segmentId string) {
	s.segmentsMutex.Lock()
	delete(s.segments, segmentId)
	s.segmentsMutex.Unlock()
}

func (s *metastoreShard) loadSegments(b *bbolt.Bucket) error {
	s.segmentsMutex.Lock()
	defer s.segmentsMutex.Unlock()
	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var md metastorev1.BlockMeta
		if err := md.UnmarshalVT(v); err != nil {
			return fmt.Errorf("failed to block %q: %w", string(k), err)
		}
		s.segments[md.Id] = &md
	}
	return nil
}

func (m *metastoreState) loadCompactionPlan(b *bbolt.Bucket, blockQueue *compactionJobBlockQueue) error {
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()

	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		if strings.HasPrefix(string(k), compactionBucketJobBlockQueuePrefix) {
			var storedBlockQueue compactionpb.CompactionJobBlockQueue
			if err := storedBlockQueue.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to load compaction job block queue %q: %w", string(k), err)
			}
			blockQueue.blocksByLevel[storedBlockQueue.CompactionLevel] = storedBlockQueue.Blocks
			level.Debug(m.logger).Log(
				"msg", "restored compaction job block queue",
				"shard", storedBlockQueue.Shard,
				"compaction_level", storedBlockQueue.CompactionLevel,
				"block_count", len(storedBlockQueue.Blocks),
				"blocks", storedBlockQueue.Blocks)
		} else {
			var job compactionpb.CompactionJob
			if err := job.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to unmarshal job %q: %w", string(k), err)
			}
			m.compactionJobQueue.enqueue(&job)
			level.Debug(m.logger).Log(
				"msg", "restored job into queue",
				"shard", job.Shard,
				"tenant", job.TenantId,
				"compaction_level", job.CompactionLevel,
				"job_status", job.Status.String(),
				"raft_log_index", job.RaftLogIndex,
				"lease_expires_at", job.LeaseExpiresAt,
				"block_count", len(job.Blocks),
				"blocks", job.Blocks)
		}
	}
	return nil
}
