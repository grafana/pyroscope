package metastore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

const (
	jobPollInterval  = 5 * time.Second
	jobLeaseDuration = 3 * jobPollInterval
)

var (
	// TODO aleks: for illustration purposes, to be moved externally
	globalCompactionStrategy = compactionStrategy{
		levels: map[uint32]compactionLevelStrategy{
			0: {maxBlocks: 20},
		},
		defaultStrategy: compactionLevelStrategy{
			maxBlocks: 10,
		},
		maxCompactionLevel: 3,
		// 0: 0.5
		// 1: 10s
		// 2: 100s
		// 3: 1000s // 16m40s
	}
)

type compactionStrategy struct {
	levels             map[uint32]compactionLevelStrategy
	defaultStrategy    compactionLevelStrategy
	maxCompactionLevel uint32
}

type compactionLevelStrategy struct {
	maxBlocks         int
	maxTotalSizeBytes uint64
}

func getStrategyForLevel(compactionLevel uint32) compactionLevelStrategy {
	strategy, ok := globalCompactionStrategy.levels[compactionLevel]
	if !ok {
		strategy = globalCompactionStrategy.defaultStrategy
	}
	return strategy
}

func (s compactionLevelStrategy) shouldCreateJob(blocks []string) bool {
	// NB: Total block size does not reflect the actual size of the data
	// to be read for compaction (at once) or queried. A better heuristic
	// would be max tenant service size.
	return len(blocks) >= s.maxBlocks
}

type compactionJobBlockQueue struct {
	mu            sync.Mutex
	blocksByLevel map[uint32][]string
}

func (m *Metastore) GetCompactionJobs(_ context.Context, req *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
	return nil, nil
}

func (m *metastoreState) tryCreateJob(block *metastorev1.BlockMeta, raftLogIndex uint64) *compactionpb.CompactionJob {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	blockQueue := m.getOrCreateCompactionBlockQueue(key)
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()

	if block.CompactionLevel >= globalCompactionStrategy.maxCompactionLevel {
		level.Info(m.logger).Log("msg", "skipping block at max compaction level", "block", block.Id, "compaction_level", block.CompactionLevel)
		return nil
	}

	queuedBlocks := append(blockQueue.blocksByLevel[block.CompactionLevel], block.Id)

	level.Debug(m.logger).Log(
		"msg", "adding block for compaction",
		"block", block.Id,
		"shard", block.Shard,
		"tenant", block.TenantId,
		"compaction_level", block.CompactionLevel,
		"size", block.Size,
		"queue_size", len(queuedBlocks),
		"raft_log_index", raftLogIndex)

	strategy := getStrategyForLevel(block.CompactionLevel)

	var job *compactionpb.CompactionJob
	if strategy.shouldCreateJob(queuedBlocks) {
		blockIds := make([]string, 0, len(queuedBlocks))
		for _, b := range queuedBlocks {
			blockIds = append(blockIds, b)
		}
		job = &compactionpb.CompactionJob{
			Name:            fmt.Sprintf("L%d-S%d-%d", block.CompactionLevel, block.Shard, calculateHash(queuedBlocks)),
			Blocks:          blockIds,
			Status:          compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED,
			Shard:           block.Shard,
			TenantId:        block.TenantId,
			CompactionLevel: block.CompactionLevel,
		}
		level.Info(m.logger).Log(
			"msg", "created compaction job",
			"job", job.Name,
			"blocks", len(queuedBlocks),
			"shard", block.Shard,
			"tenant", block.TenantId,
			"compaction_level", block.CompactionLevel)
	}
	return job
}

func (m *metastoreState) addCompactionJob(job *compactionpb.CompactionJob) {
	level.Debug(m.logger).Log("msg", "adding compaction job to priority queue", "job", job.Name)
	if ok := m.compactionJobQueue.enqueue(job); !ok {
		level.Warn(m.logger).Log("msg", "a compaction job with this name already exists", "job", job.Name)
		return
	}

	// reset the pre-queue for this level
	key := tenantShard{
		tenant: job.TenantId,
		shard:  job.Shard,
	}
	blockQueue := m.getOrCreateCompactionBlockQueue(key)
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()
	blockQueue.blocksByLevel[job.CompactionLevel] = blockQueue.blocksByLevel[job.CompactionLevel][:0]
}

func (m *metastoreState) addBlockToCompactionJobQueue(block *metastorev1.BlockMeta) {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	blockQueue := m.getOrCreateCompactionBlockQueue(key)
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()

	blockQueue.blocksByLevel[block.CompactionLevel] = append(blockQueue.blocksByLevel[block.CompactionLevel], block.Id)
}

func calculateHash(blocks []string) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk...)
	}
	return xxhash.Sum64(b)
}

type compactionMetrics struct {
	addedBlocks   *prometheus.CounterVec
	deletedBlocks *prometheus.CounterVec
	addedJobs     *prometheus.CounterVec
	assignedJobs  *prometheus.CounterVec
	completedJobs *prometheus.CounterVec
}

func newCompactionMetrics(reg prometheus.Registerer) *compactionMetrics {
	m := &compactionMetrics{
		addedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_added_blocks_count",
			Help:      "The number of blocks added for compaction",
		}, []string{"shard", "tenant", "level"}),
		deletedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_deleted_blocks_count",
			Help:      "The number of blocks deleted as a result of compaction",
		}, []string{"shard", "tenant", "level"}),
		addedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_added_jobs_count",
			Help:      "The number of created compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		assignedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_assigned_jobs_count",
			Help:      "The number of assigned compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		completedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_completed_jobs_count",
			Help:      "The number of completed compaction jobs",
		}, []string{"shard", "tenant", "level"}),
	}
	if reg != nil {
		reg.MustRegister(
			m.addedBlocks,
			m.deletedBlocks,
			m.addedJobs,
			m.assignedJobs,
			m.completedJobs,
		)
	}
	return m
}

func (m *metastoreState) consumeBlock(block *metastorev1.BlockMeta, tx *bbolt.Tx, raftLogIndex uint64) (err error, jobToAdd *compactionpb.CompactionJob, blockForQueue *metastorev1.BlockMeta) {
	// create and store an optional compaction job
	if job := m.tryCreateJob(block, raftLogIndex); job != nil {
		level.Debug(m.logger).Log("msg", "persisting compaction job", "job", job.Name)
		jobBucketName, jobKey := keyForCompactionJob(block.Shard, block.TenantId, job.Name)
		err := updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
			data, _ := job.MarshalVT()
			return bucket.Put(jobKey, data)
		})
		if err != nil {
			return err, nil, nil
		}
		err = m.persistCompactionJobBlockQueue(block.Shard, block.TenantId, block.CompactionLevel, []string{}, tx)
		jobToAdd = job
	} else {
		key := tenantShard{
			tenant: block.TenantId,
			shard:  block.Shard,
		}
		queue := m.getOrCreateCompactionBlockQueue(key).blocksByLevel[block.CompactionLevel]
		queue = append(queue, block.Id)
		err := m.persistCompactionJobBlockQueue(block.Shard, block.TenantId, block.CompactionLevel, queue, tx)
		if err != nil {
			return err, nil, nil
		}
		blockForQueue = block
	}
	return err, jobToAdd, blockForQueue
}

func (m *metastoreState) persistCompactionJobBlockQueue(shard uint32, tenant string, compactionLevel uint32, queue []string, tx *bbolt.Tx) error {
	jobBucketName, _ := keyForCompactionJob(shard, tenant, "")
	blockQueue := &compactionpb.CompactionJobBlockQueue{
		CompactionLevel: compactionLevel,
		Shard:           shard,
		Tenant:          tenant,
		Blocks:          queue,
	}
	key := []byte(fmt.Sprintf("job-pre-queue-%d", compactionLevel))
	return updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
		data, _ := blockQueue.MarshalVT()
		return bucket.Put(key, data)
	})
}
