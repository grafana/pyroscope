package metastore

import (
	"context"
	"fmt"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type compactionPlan struct {
	jobsMutex           sync.Mutex
	jobsByName          map[string]*compactorv1.CompactionJob
	queuedBlocksByLevel map[uint32][]*metastorev1.BlockMeta
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

func (m *Metastore) UpdateJobStatus(_ context.Context, req *compactorv1.UpdateJobStatusRequest) (*compactorv1.UpdateJobStatusResponse, error) {
	_, resp, err := applyCommand[*compactorv1.UpdateJobStatusRequest, *compactorv1.UpdateJobStatusResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	return resp, err
}

func (m *metastoreState) addForCompaction(block *metastorev1.BlockMeta) *compactorv1.CompactionJob {
	plan := m.getOrCreatePlan(block.Shard)
	plan.jobsMutex.Lock()
	defer plan.jobsMutex.Unlock()

	plan.queuedBlocksByLevel[block.CompactionLevel] = append(plan.queuedBlocksByLevel[block.CompactionLevel], block)
	queuedBlocks := plan.queuedBlocksByLevel[block.CompactionLevel]

	var job *compactorv1.CompactionJob
	if len(queuedBlocks) >= 10 { // TODO aleks: add block size sum to the condition
		job = &compactorv1.CompactionJob{
			Name:   fmt.Sprintf("L%d-S%d-%d", block.CompactionLevel, block.Shard, calculateHash(queuedBlocks)),
			Blocks: queuedBlocks,
			Status: &compactorv1.CompactionJobStatus{
				Status: compactorv1.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED,
			},
		}
		plan.jobsByName[job.Name] = job
		plan.queuedBlocksByLevel[block.CompactionLevel] = plan.queuedBlocksByLevel[block.CompactionLevel][:0]
	}
	return job
}

func calculateHash(blocks []*metastorev1.BlockMeta) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk.Id...)
	}
	return xxhash.Sum64(b)
}
