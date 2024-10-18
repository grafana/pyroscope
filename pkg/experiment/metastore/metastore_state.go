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

	"github.com/grafana/pyroscope/pkg/experiment/metastore/blockcleaner"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
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
	reg               prometheus.Registerer
	compactionMetrics *compactionMetrics
	compactionConfig  *CompactionConfig

	index       *index.Index
	indexConfig *index.Config

	deletionMarkers *blockcleaner.DeletionMarkers
	blockCleaner    *blockCleaner

	compactionMutex          sync.Mutex
	compactionJobBlockQueues map[tenantShard]*compactionJobBlockQueue
	compactionJobQueue       *jobQueue

	db *boltdb
}

type compactionJobBlockQueue struct {
	mu            sync.Mutex
	blocksByLevel map[uint32][]string
}

func newMetastoreState(
	logger log.Logger,
	db *boltdb,
	reg prometheus.Registerer,
	compactionCfg *CompactionConfig,
	indexCfg *index.Config,
) *metastoreState {
	return &metastoreState{
		logger:                   logger,
		reg:                      reg,
		index:                    index.NewIndex(newIndexStore(db, logger), logger, indexCfg),
		db:                       db,
		compactionJobBlockQueues: make(map[tenantShard]*compactionJobBlockQueue),
		compactionJobQueue:       newJobQueue(compactionCfg.JobLeaseDuration.Nanoseconds()),
		compactionMetrics:        newCompactionMetrics(reg),
		compactionConfig:         compactionCfg,
		indexConfig:              indexCfg,
	}
}

func (m *metastoreState) reset(db *boltdb) {
	m.compactionMutex.Lock()
	defer m.compactionMutex.Unlock()
	clear(m.compactionJobBlockQueues)
	m.index = index.NewIndex(newIndexStore(db, m.logger), m.logger, m.indexConfig)
	m.compactionJobQueue = newJobQueue(m.compactionConfig.JobLeaseDuration.Nanoseconds())
	m.db = db
	m.deletionMarkers.Reload(db.boltdb)
}

func (m *metastoreState) restore(db *boltdb) error {
	m.reset(db)
	m.index.LoadPartitions()
	return db.boltdb.View(func(tx *bbolt.Tx) error {
		return m.restoreCompactionPlan(tx)
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
				"blocks", strings.Join(storedBlockQueue.Blocks, ","))
		} else {
			var job compactionpb.CompactionJob
			if err := job.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to unmarshal job %q: %w", string(k), err)
			}
			m.compactionJobQueue.enqueue(&job)
			level.Debug(m.logger).Log(
				"msg", "restored job into queue",
				"job", job.Name,
				"shard", job.Shard,
				"tenant", job.TenantId,
				"compaction_level", job.CompactionLevel,
				"job_status", job.Status.String(),
				"raft_log_index", job.RaftLogIndex,
				"lease_expires_at", job.LeaseExpiresAt,
				"block_count", len(job.Blocks),
				"blocks", strings.Join(job.Blocks, ","))
		}
	}
	return nil
}
