package metastore

import (
	"context"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
)

// TODO(kolesnikovae): Validate input.
// TODO(kolesnikovae): Instrument with metrics.

type CompactionService struct {
	metastorev1.CompactionServiceServer

	logger log.Logger
	raft   Raft
	mu     sync.Mutex
}

func NewCompactionService(
	logger log.Logger,
	raft Raft,
) *CompactionService {
	return &CompactionService{
		logger: logger,
		raft:   raft,
	}
}

func (m *CompactionService) PollCompactionJobs(
	_ context.Context,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	// This is a two-step process. To commit changes to the compaction plan,
	// we need to ensure that all replicas apply exactly the same changes.
	// Instead of relying on identical behavior across replicas and a
	// reproducible compaction plan, we explicitly replicate the change.
	//
	// NOTE(kolesnikovae): We can use Leader Read optimization here. However,
	// we would need to ensure synchronization between the compactor and the
	// index. For now, the read operation is conducted through the raft log.
	//
	// Make sure that only one compaction plan update is in progress at a time.
	// This lock does not introduce contention, as the raft log is synchronous.
	m.mu.Lock()
	defer m.mu.Unlock()

	// First, we ask the current leader to prepare the change. This is a read
	// operation conducted through the raft log: at this stage, we only
	// prepare changes; the command handler does not alter the state.
	request := &raft_log.GetCompactionPlanUpdateRequest{
		StatusUpdates: make([]*metastorev1.CompactionJobStatusUpdate, 0, len(req.StatusUpdates)),
		AssignJobsMax: req.JobCapacity,
	}
	// We do not send the compacted blocks to the raft log to minimize the
	// replication traffic. In fact, the compacted blocks are not needed for
	// planning at this point.
	for _, update := range req.StatusUpdates {
		request.StatusUpdates = append(request.StatusUpdates, &metastorev1.CompactionJobStatusUpdate{
			Name:            update.Name,
			Token:           update.Token,
			Status:          update.Status,
			CompactedBlocks: update.CompactedBlocks, // TODO: Remove.
		})
	}

	planUpdate, err := proposeGetCompactionPlanUpdate(m.raft, request)
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to prepare compaction plan", "err", err)
		return nil, err
	}

	// Copy plan updates to the worker response.
	resp := &metastorev1.PollCompactionJobsResponse{
		CompactionJobs: make([]*metastorev1.CompactionJob, 0, len(planUpdate.AssignedJobs)),
		Assignments:    make([]*metastorev1.CompactionJobAssignment, 0, len(planUpdate.AssignedJobs)),
	}
	for _, assigned := range planUpdate.AssignedJobs {
		assignment := assigned.State
		resp.Assignments = append(resp.Assignments, &metastorev1.CompactionJobAssignment{
			Name:           assignment.Name,
			Token:          assignment.Token,
			LeaseExpiresAt: assignment.LeaseExpiresAt,
			Status:         assignment.Status,
		})
		// The job plan is only sent for newly assigned jobs.
		// Lease renewals do not require the plan to be sent.
		if assigned.Plan != nil {
			job := assigned.Plan
			resp.CompactionJobs = append(resp.CompactionJobs, &metastorev1.CompactionJob{
				Name:            job.Name,
				Shard:           job.Shard,
				Tenant:          job.Tenant,
				CompactionLevel: job.CompactionLevel,
				SourceBlocks:    job.SourceBlocks,
				Tombstones:      job.Tombstones,
			})
		}
	}

	// Assigned jobs are not written to the raft log (only the assignments).
	// The plan has already been proposed and accepted in the past when the job
	// was created.
	for _, job := range planUpdate.AssignedJobs {
		job.Plan = nil
	}

	// Now that we have the plan, we need to propagate it through the raft log
	// to ensure it is applied consistently across all replicas, regardless of
	// their individual state or view of the plan.
	proposal := &raft_log.UpdateCompactionPlanRequest{PlanUpdate: planUpdate}
	if err = proposeCompactionPlanUpdate(m.raft, proposal); err != nil {
		level.Error(m.logger).Log("msg", "failed to update compaction plan", "err", err)
		return nil, err
	}

	return resp, nil
}

func proposeGetCompactionPlanUpdate(raft Raft, req *raft_log.GetCompactionPlanUpdateRequest) (*raft_log.CompactionPlanUpdate, error) {
	resp, err := raft.Propose(fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_GET_COMPACTION_PLAN_UPDATE), req)
	if err != nil {
		return nil, err
	}
	return resp.(*raft_log.GetCompactionPlanUpdateResponse).GetPlanUpdate(), nil
}

func proposeCompactionPlanUpdate(raft Raft, req *raft_log.UpdateCompactionPlanRequest) error {
	_, err := raft.Propose(fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN), req)
	return err
}
