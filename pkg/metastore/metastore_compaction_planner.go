package metastore

import (
	"context"
	"fmt"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type compactionPlan struct {
	jobsMutex  sync.Mutex
	jobsByName map[string]*compactorv1.CompactionJob

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

func (m *Metastore) addForCompaction(block *metastorev1.BlockMeta) {
	plan := m.state.getOrCreatePlan(block.Shard)
	plan.jobsMutex.Lock()
	defer plan.jobsMutex.Unlock()

	queuedBlocks := plan.queuedBlocksByLevel[block.CompactionLevel]
	if queuedBlocks == nil {
		queuedBlocks = make([]*metastorev1.BlockMeta, 0)

	}
	queuedBlocks = append(queuedBlocks, block)

	name, key := keyForCompactionBlockQueue(block.Shard, block.TenantId, block.CompactionLevel)
	value := &compactorv1.BlockMetas{Blocks: queuedBlocks}
	err := m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		return updateCompactionPlanBucket(tx, name, func(bucket *bbolt.Bucket) error {
			data, _ := value.MarshalVT()
			return bucket.Put(key, data)
		})
	})
	if err != nil {
		m.logger.Log("msg", "failed to update queued blocks for compaction in the bucket", "err", err)
		return
	}

	var job *compactorv1.CompactionJob
	if len(queuedBlocks) >= 10 { // TODO aleks: add block size sum to the condition
		job = &compactorv1.CompactionJob{
			Name:    fmt.Sprintf("L%d-S%d-%d", block.CompactionLevel, block.Shard, StableHash(queuedBlocks)),
			Options: nil,
			Blocks:  queuedBlocks,
			Status: &compactorv1.CompactionJobStatus{
				Status: compactorv1.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED,
			},
		}

		name, key := keyForCompactionJob(block.Shard, block.TenantId, job.Name)
		err := m.db.boltdb.Update(func(tx *bbolt.Tx) error {
			return updateCompactionPlanBucket(tx, name, func(bucket *bbolt.Bucket) error {
				data, _ := job.MarshalVT()
				return bucket.Put(key, data)
			})
		})
		if err != nil {
			m.logger.Log("msg", "failed to store compaction job in the bucket", "err", err, "job", job)
			return
		}

		queuedBlocks = queuedBlocks[:0]
	}

	if job != nil {
		plan.jobsByName[job.Name] = job
	}
	plan.queuedBlocksByLevel[block.CompactionLevel] = queuedBlocks
}

func StableHash(blocks []*metastorev1.BlockMeta) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk.Id...)
	}
	return xxhash.Sum64(b)
}
