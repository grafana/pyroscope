package metastore

import (
	"context"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

// TODO(kolesnikovae): Validate input.

// TODO(kolesnikovae): Instrument with metrics.

type CompactionPlanLog interface {
	GetCompactionPlanUpdate(*raft_log.GetCompactionPlanUpdateRequest) (*raft_log.GetCompactionPlanUpdateResponse, error)
	UpdateCompactionPlan(*raft_log.UpdateCompactionPlanRequest) (*raft_log.UpdateCompactionPlanResponse, error)
}

func NewCompactionService(
	logger log.Logger,
	raftLog CompactionPlanLog,
) *CompactionService {
	return &CompactionService{
		logger: logger,
		raft:   raftLog,
	}
}

type CompactionService struct {
	logger log.Logger
	mu     sync.Mutex
	raft   CompactionPlanLog
}

func (m *CompactionService) PollCompactionJobs(
	_ context.Context,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	request := &raft_log.GetCompactionPlanUpdateRequest{
		StatusUpdates:       req.StatusUpdates,
		AssignJobsMax:       req.JobCapacity,
		DeleteTombstonesMax: req.CleanupCapacity,
	}

	// This is a two-step process. To commit changes to the compaction plan,
	// we need to ensure that all replicas apply exactly the same changes.
	// Instead of relying on identical behavior across replicas and a
	// reproducible compaction plan, we explicitly replicate the change.

	// Make sure that only one compaction plan update is in progress at a time.
	// This lock does not introduce contention, as the raft log is synchronous.
	m.mu.Lock()
	defer m.mu.Unlock()

	// First, we ask the current leader to prepare the change. This is a read
	// operation conducted through the raft log: at this stage, we only
	// prepare changes; the command handler does not alter the state.
	prepared, err := m.raft.GetCompactionPlanUpdate(request)
	if err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to prepare compaction plan", "err", err)
		return nil, err
	}

	// Now that we have the plan, we need to propagate it through the raft log
	// to ensure it is applied consistently across all replicas, regardless of
	// their individual state or view of the plan.
	proposal := &raft_log.UpdateCompactionPlanRequest{PlanUpdate: prepared.PlanUpdate}
	if _, err = m.raft.UpdateCompactionPlan(proposal); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to update compaction plan", "err", err)
		return nil, err
	}

	// The response we sent to the worker differs from the compaction plan: the
	// latter contains all state transitions including terminal ones.
	resp := &metastorev1.PollCompactionJobsResponse{
		CompactionJobs: make([]*metastorev1.CompactionJob, 0, len(prepared.PlanUpdate.CompactionJobs)),
		Assignments:    make([]*metastorev1.CompactionJobAssignment, 0, len(prepared.PlanUpdate.JobUpdates)),
	}

	requested := make(map[string]*metastorev1.CompactionJobStatusUpdate, len(request.StatusUpdates))
	for _, update := range request.StatusUpdates {
		requested[update.Name] = update
	}

	for _, update := range prepared.PlanUpdate.JobUpdates {
		if update.Status != metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS {
			// Other statuses are not sent to workers.
			continue
		}
		// If the fencing token of the job is greater than the token of the
		// worker, the job has been revoked and assigned to another instance.
		// We should signal the worker to cancel the job, as we won't accept
		// any results.
		if requested[update.Name].Token < update.RaftLogIndex {
			resp.Assignments = append(resp.Assignments, &metastorev1.CompactionJobAssignment{
				JobStatus: metastorev1.CompactionJobStatus_COMPACTION_STATUS_CANCELLED,
			})
			continue
		}
		resp.Assignments = append(resp.Assignments, &metastorev1.CompactionJobAssignment{
			Token:          update.RaftLogIndex,
			LeaseExpiresAt: update.LeaseExpiresAt,
			JobStatus:      update.Status,
		})
	}

	// All newly created and re-assigned compaction jobs are sent to workers.
	for _, job := range prepared.PlanUpdate.CompactionJobs {
		resp.CompactionJobs = append(resp.CompactionJobs, &metastorev1.CompactionJob{
			Name:            job.Name,
			Shard:           job.Shard,
			Tenant:          job.Tenant,
			CompactionLevel: job.CompactionLevel,
			SourceBlocks:    job.SourceBlocks,
			Tombstones:      job.Tombstones,
		})
	}

	return resp, nil
}
