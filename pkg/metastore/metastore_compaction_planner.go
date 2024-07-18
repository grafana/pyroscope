package metastore

import (
	"context"
	"fmt"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
)

var (
	// TODO aleks: for illustration purposes, to be moved externally
	globalCompactionStrategy = compactionStrategy{
		levels: map[uint32]compactionLevelStrategy{
			0: {
				minBlocks:         10,
				maxBlocks:         20,
				minTotalSizeBytes: 2 << 18, // 512KB
				maxTotalSizeBytes: 2 << 20, // 2MB
			},
		},
		defaultStrategy: compactionLevelStrategy{
			minBlocks:         10,
			maxBlocks:         0,
			minTotalSizeBytes: 0,
			maxTotalSizeBytes: 0,
		},
		maxCompactionLevel: 10,
	}
)

type compactionStrategy struct {
	levels             map[uint32]compactionLevelStrategy
	defaultStrategy    compactionLevelStrategy
	maxCompactionLevel uint32
}

type compactionLevelStrategy struct {
	minBlocks         int
	maxBlocks         int
	minTotalSizeBytes uint64
	maxTotalSizeBytes uint64
}

func getStrategyForLevel(compactionLevel uint32) compactionLevelStrategy {
	strategy, ok := globalCompactionStrategy.levels[compactionLevel]
	if !ok {
		strategy = globalCompactionStrategy.defaultStrategy
	}
	return strategy
}

func (s compactionLevelStrategy) shouldCreateJob(blocks []*metastorev1.BlockMeta) bool {
	totalSizeBytes := getTotalSize(blocks)
	enoughBlocks := len(blocks) >= s.minBlocks
	enoughData := totalSizeBytes > 0 && totalSizeBytes >= s.minTotalSizeBytes
	if enoughBlocks && enoughData {
		return true
	} else if enoughBlocks {
		return s.maxBlocks > 0 && len(blocks) >= s.maxBlocks
	} else if enoughData {
		return s.maxTotalSizeBytes > 0 && totalSizeBytes >= s.maxTotalSizeBytes
	}
	return false
}

type Planner struct {
	logger         log.Logger
	metastoreState *metastoreState
}

func NewPlanner(state *metastoreState, logger log.Logger) *Planner {
	return &Planner{
		metastoreState: state,
		logger:         logger,
	}
}

type jobPreQueue struct {
	mu            sync.Mutex
	blocksByLevel map[uint32][]*metastorev1.BlockMeta
}

func (m *Metastore) GetCompactionJobs(_ context.Context, req *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
	return nil, nil
}

func (m *metastoreState) tryCreateJob(block *metastorev1.BlockMeta) *compactionpb.CompactionJob {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	preQueue := m.getOrCreatePreQueue(key)
	preQueue.mu.Lock()
	defer preQueue.mu.Unlock()

	if block.CompactionLevel > globalCompactionStrategy.maxCompactionLevel {
		level.Info(m.logger).Log("msg", "skipping block at max compaction level", "block", block.Id, "compaction_level", block.CompactionLevel)
		return nil
	}

	queuedBlocks := append(preQueue.blocksByLevel[block.CompactionLevel], block)

	level.Debug(m.logger).Log(
		"msg", "adding block for compaction",
		"block", block.Id,
		"shard", block.Shard,
		"tenant", block.TenantId,
		"compaction_level", block.CompactionLevel,
		"size", block.Size,
		"queue_size", len(queuedBlocks))

	strategy := getStrategyForLevel(block.CompactionLevel)

	var job *compactionpb.CompactionJob
	if strategy.shouldCreateJob(queuedBlocks) {
		blockIds := make([]string, 0, len(queuedBlocks))
		for _, block := range queuedBlocks {
			blockIds = append(blockIds, block.Id)
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
			"blocks_bytes", getTotalSize(queuedBlocks),
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
	preQueue := m.getOrCreatePreQueue(key)
	preQueue.mu.Lock()
	defer preQueue.mu.Unlock()
	preQueue.blocksByLevel[job.CompactionLevel] = preQueue.blocksByLevel[job.CompactionLevel][:0]
}

func (m *metastoreState) addBlockToCompactionJobQueue(block *metastorev1.BlockMeta) {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	preQueue := m.getOrCreatePreQueue(key)
	preQueue.mu.Lock()
	defer preQueue.mu.Unlock()

	preQueue.blocksByLevel[block.CompactionLevel] = append(preQueue.blocksByLevel[block.CompactionLevel], block)
}

func getTotalSize(blocks []*metastorev1.BlockMeta) uint64 {
	totalSizeBytes := uint64(0)
	for _, block := range blocks {
		totalSizeBytes += block.Size
	}
	return totalSizeBytes
}

func calculateHash(blocks []*metastorev1.BlockMeta) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk.Id...)
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
