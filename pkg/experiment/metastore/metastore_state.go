package metastore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/blockcleaner"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type metastoreState struct {
	logger log.Logger
	reg    prometheus.Registerer
	db     *boltdb

	// TODO(kolesnikovae): Refactor out.
	compactionMutex          sync.Mutex
	compactionJobBlockQueues map[tenantShard]*compactionJobBlockQueue
	compactionJobQueue       *jobQueue
	compactionMetrics        *compactionMetrics
	compactionConfig         *CompactionConfig

	// TODO(kolesnikovae): Refactor out.
	index       *index.Index
	indexConfig *index.Config

	// TODO(kolesnikovae): Refactor out.
	deletionMarkers *blockcleaner.DeletionMarkers
	blockCleaner    *blockCleaner
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
		logger: logger,
		reg:    reg,
		db:     db,

		compactionConfig:         compactionCfg,
		compactionJobBlockQueues: make(map[tenantShard]*compactionJobBlockQueue),
		compactionJobQueue:       newJobQueue(compactionCfg.JobLeaseDuration.Nanoseconds()),
		compactionMetrics:        newCompactionMetrics(reg),

		indexConfig: indexCfg,
		index:       index.NewIndex(newIndexStore(db, logger), logger, indexCfg),
	}
}

func (m *metastoreState) Restore(snapshot io.ReadCloser) error {
	t1 := time.Now()
	_ = level.Info(m.logger).Log("msg", "restoring snapshot")
	defer func() {
		_ = snapshot.Close()
		m.db.metrics.fsmRestoreSnapshotDuration.Observe(time.Since(t1).Seconds())
	}()
	if err := m.db.restore(snapshot); err != nil {
		return fmt.Errorf("failed to restore from snapshot: %w", err)
	}
	// First, clear the state.
	clear(m.compactionJobBlockQueues)
	m.compactionJobQueue = newJobQueue(m.compactionConfig.JobLeaseDuration.Nanoseconds())
	m.index = index.NewIndex(newIndexStore(m.db, m.logger), m.logger, m.indexConfig)
	// Now load data from db.
	// TODO(kolesnikovae): These should return any error encountered.
	m.deletionMarkers.Reload(m.db.boltdb)
	m.index.LoadPartitions()
	return m.db.boltdb.View(m.restoreCompactionPlan)
}

func (m *metastoreState) Snapshot() (raft.FSMSnapshot, error) {
	// Snapshot should only capture a pointer to the state, and any
	// expensive IO should happen as part of FSMSnapshot.Persist.
	s := snapshot{logger: m.db.logger, metrics: m.db.metrics}
	tx, err := m.db.boltdb.Begin(false)
	if err != nil {
		return nil, fmt.Errorf("failed to open a transaction for snapshot: %w", err)
	}
	s.tx = tx
	return &s, nil
}

type snapshot struct {
	logger  log.Logger
	tx      *bbolt.Tx
	metrics *metastoreMetrics
}

func (s *snapshot) Persist(sink raft.SnapshotSink) (err error) {
	pprof.Do(context.Background(), pprof.Labels("metastore_op", "persist"), func(ctx context.Context) {
		err = s.persist(sink)
	})
	return err
}

func (s *snapshot) persist(sink raft.SnapshotSink) (err error) {
	start := time.Now()
	_ = s.logger.Log("msg", "persisting snapshot", "sink_id", sink.ID())
	defer func() {
		s.metrics.boltDBPersistSnapshotDuration.Observe(time.Since(start).Seconds())
		s.logger.Log("msg", "persisted snapshot", "sink_id", sink.ID(), "err", err, "duration", time.Since(start))
		if err != nil {
			_ = s.logger.Log("msg", "failed to persist snapshot", "err", err)
			if err = sink.Cancel(); err != nil {
				_ = s.logger.Log("msg", "failed to cancel snapshot sink", "err", err)
				return
			}
		}
		if err = sink.Close(); err != nil {
			_ = s.logger.Log("msg", "failed to close sink", "err", err)
		}
	}()
	_ = level.Info(s.logger).Log("msg", "persisting snapshot")
	if _, err = s.tx.WriteTo(sink); err != nil {
		_ = level.Error(s.logger).Log("msg", "failed to write snapshot", "err", err)
		return err
	}
	return nil
}

func (s *snapshot) Release() {
	if s.tx != nil {
		// This is an in-memory rollback, no error expected.
		_ = s.tx.Rollback()
	}
}

// TODO(kolesnikovae): Refactor out ---------------------------------------------------------------

const (
	compactionBucketJobBlockQueuePrefix = "compaction-job-block-queue"
)

type tenantShard struct {
	tenant string
	shard  uint32
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
