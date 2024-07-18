package metastore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
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
	compactionPlans      map[tenantShard]*compactionPlan

	db *boltdb
}

type metastoreShard struct {
	segmentsMutex sync.Mutex
	segments      map[string]*metastorev1.BlockMeta
}

func newMetastoreState(logger log.Logger, db *boltdb, reg prometheus.Registerer) *metastoreState {
	return &metastoreState{
		logger:            logger,
		shards:            make(map[uint32]*metastoreShard),
		db:                db,
		compactionPlans:   make(map[tenantShard]*compactionPlan),
		compactionMetrics: newCompactionMetrics(reg),
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
		shardId, tenant, ok := parseBucketName(name)
		if !ok {
			_ = level.Error(m.logger).Log("msg", "malformed bucket name", "name", string(name))
			return nil
		}
		key := tenantShard{
			tenant: tenant,
			shard:  shardId,
		}
		planForShard := m.getOrCreatePlan(key)
		return planForShard.loadJobs(cdb.Bucket(name))
	})

}

func (m *metastoreState) getOrCreatePlan(key tenantShard) *compactionPlan {
	m.compactionPlansMutex.Lock()
	defer m.compactionPlansMutex.Unlock()

	if plan, ok := m.compactionPlans[key]; ok {
		return plan
	}
	plan := &compactionPlan{
		jobsByName:          make(map[string]*compactionpb.CompactionJob),
		queuedBlocksByLevel: make(map[uint32][]*metastorev1.BlockMeta),
	}
	m.compactionPlans[key] = plan
	return plan
}

func (m *metastoreState) findJob(key tenantShard, name string) *compactionpb.CompactionJob {
	plan := m.getOrCreatePlan(key)
	plan.jobsMutex.Lock()
	defer plan.jobsMutex.Unlock()

	return plan.jobsByName[name]
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

func (p *compactionPlan) loadJobs(b *bbolt.Bucket) error {
	p.jobsMutex.Lock()
	defer p.jobsMutex.Unlock()
	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var job compactionpb.CompactionJob
		if err := job.UnmarshalVT(v); err != nil {
			return fmt.Errorf("failed to unmarshal job %q: %w", string(k), err)
		}
		p.jobsByName[job.Name] = &job
		// TODO aleks: restoring from a snapshot will lose "partial" jobs
	}
	return nil
}

func (m *metastoreState) getJobs(status compactionpb.CompactionStatus, fn func(job *compactionpb.CompactionJob) (exit bool)) <-chan *compactionpb.CompactionJob {
	ch := make(chan *compactionpb.CompactionJob)
	go func() {
		defer close(ch)

		m.compactionPlansMutex.Lock()
		defer m.compactionPlansMutex.Unlock()

		for _, plan := range m.compactionPlans {
			plan.jobsMutex.Lock()
			for _, job := range plan.jobsByName {
				if job.Status != status {
					continue
				}
				exitCondition := fn(job)
				if exitCondition {
					plan.jobsMutex.Unlock()
					return
				}
				ch <- job
			}
			plan.jobsMutex.Unlock()
		}
	}()

	return ch
}
