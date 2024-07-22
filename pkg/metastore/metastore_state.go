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
	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
)

const (
	compactionBucketJobPreQueuePrefix = "job-pre-queue"
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

	compactionPlansMutex sync.Mutex
	preCompactionQueues  map[tenantShard]*jobPreQueue
	compactionJobQueue   *jobQueue

	db *boltdb
}

type metastoreShard struct {
	segmentsMutex sync.Mutex
	segments      map[string]*metastorev1.BlockMeta
}

func newMetastoreState(logger log.Logger, db *boltdb, reg prometheus.Registerer) *metastoreState {
	return &metastoreState{
		logger:              logger,
		shards:              make(map[uint32]*metastoreShard),
		db:                  db,
		preCompactionQueues: make(map[tenantShard]*jobPreQueue),
		compactionJobQueue:  newJobQueue(jobLeaseDuration.Nanoseconds()),
		compactionMetrics:   newCompactionMetrics(reg),
	}
}

func (m *metastoreState) reset() {
	m.shardsMutex.Lock()
	clear(m.shards)
	m.shardsMutex.Unlock()
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
	m.reset()
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
		shardID, tenantID, ok := parseBucketName(name)
		if !ok {
			_ = level.Error(m.logger).Log("msg", "malformed bucket name", "name", string(name))
			return nil
		}
		shard := m.getOrCreateShard(shardID)
		if tenantID != "" {
			_ = level.Debug(m.logger).Log("compacted blocks are ignored")
			// TODO: Load tenant blocks.
			return nil
		}
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
		m.compactionPlansMutex.Lock()
		defer m.compactionPlansMutex.Unlock()
		preQueue := m.getOrCreatePreQueue(key)

		return m.loadCompactionPlan(cdb.Bucket(name), preQueue)
	})

}

func (m *metastoreState) getOrCreatePreQueue(key tenantShard) *jobPreQueue {
	m.compactionPlansMutex.Lock()
	defer m.compactionPlansMutex.Unlock()

	if preQueue, ok := m.preCompactionQueues[key]; ok {
		return preQueue
	}
	plan := &jobPreQueue{
		blocksByLevel: make(map[uint32][]string),
	}
	m.preCompactionQueues[key] = plan
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

func (s *metastoreShard) deleteSegment(segment *metastorev1.BlockMeta) {
	s.segmentsMutex.Lock()
	delete(s.segments, segment.Id)
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

func (m *metastoreState) loadCompactionPlan(b *bbolt.Bucket, preQueue *jobPreQueue) error {
	preQueue.mu.Lock()
	defer preQueue.mu.Unlock()

	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		if strings.HasPrefix(string(k), compactionBucketJobPreQueuePrefix) {
			var storedPreQueue compactionpb.JobPreQueue
			if err := storedPreQueue.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to load job pre queue %q: %w", string(k), err)
			}
			preQueue.blocksByLevel[storedPreQueue.CompactionLevel] = storedPreQueue.Blocks
		} else {
			var job compactionpb.CompactionJob
			if err := job.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to unmarshal job %q: %w", string(k), err)
			}
			m.compactionJobQueue.enqueue(&job)
		}
	}
	return nil
}
