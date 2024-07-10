package metastore

import (
	"context"
	"fmt"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
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

type compactionPlan struct {
	jobsMutex           sync.Mutex
	jobsByName          map[string]*compactorv1.CompactionJob
	queuedBlocksByLevel map[uint32][]*metastorev1.BlockMeta
}

func (m *Metastore) GetCompactionJobs(_ context.Context, req *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
	m.state.compactionPlansMutex.Lock()
	defer m.state.compactionPlansMutex.Unlock()

	resp := &compactorv1.GetCompactionResponse{
		CompactionJobs: make([]*compactorv1.CompactionJob, 0, len(m.state.compactionPlans)),
	}
	for _, plan := range m.state.compactionPlans {
		for _, job := range plan.jobsByName {
			resp.CompactionJobs = append(resp.CompactionJobs, job)
		}
	}

	return resp, nil
}

func (m *metastoreState) tryCreateJob(block *metastorev1.BlockMeta) *compactorv1.CompactionJob {
	plan := m.getOrCreatePlan(block.Shard)
	plan.jobsMutex.Lock()
	defer plan.jobsMutex.Unlock()

	if block.CompactionLevel > globalCompactionStrategy.maxCompactionLevel {
		level.Info(m.logger).Log("msg", "skipping block at max compaction level", "block", block.Id, "compaction_level", block.CompactionLevel)
		return nil
	}

	queuedBlocks := append(plan.queuedBlocksByLevel[block.CompactionLevel], block)

	level.Debug(m.logger).Log(
		"msg", "adding block for compaction",
		"block", block.Id,
		"shard", block.Shard,
		"tenant", block.TenantId,
		"compaction_level", block.CompactionLevel,
		"size", block.Size,
		"queue_size", len(queuedBlocks))

	strategy := getStrategyForLevel(block.CompactionLevel)

	var job *compactorv1.CompactionJob
	if strategy.shouldCreateJob(queuedBlocks) {
		job = &compactorv1.CompactionJob{
			Name:    fmt.Sprintf("L%d-S%d-%d", block.CompactionLevel, block.Shard, calculateHash(queuedBlocks)),
			Options: &compactorv1.CompactionOptions{},
			Blocks:  queuedBlocks,
			Status: &compactorv1.CompactionJobStatus{
				Status: compactorv1.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED,
				CompletedJob: &compactorv1.CompletedJob{
					Blocks: make([]*metastorev1.BlockMeta, 0),
				},
				Shard:    block.Shard,
				TenantId: block.TenantId,
			},
			Shard:    block.Shard,
			TenantId: block.TenantId,
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

func (m *metastoreState) addCompactionJob(job *compactorv1.CompactionJob) {
	plan := m.getOrCreatePlan(job.Shard)
	plan.jobsMutex.Lock()
	defer plan.jobsMutex.Unlock()

	plan.jobsByName[job.Name] = job
	compactionLevel := job.Blocks[0].CompactionLevel
	plan.queuedBlocksByLevel[compactionLevel] = plan.queuedBlocksByLevel[compactionLevel][:0]
}

func (m *metastoreState) addBlockToCompactionJobQueue(block *metastorev1.BlockMeta) {
	plan := m.getOrCreatePlan(block.Shard)
	plan.jobsMutex.Lock()
	defer plan.jobsMutex.Unlock()

	plan.queuedBlocksByLevel[block.CompactionLevel] = append(plan.queuedBlocksByLevel[block.CompactionLevel], block)
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
