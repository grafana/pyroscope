package metastore

import (
	"context"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
)

type CompactionService struct {
	metastorev1.CompactionServiceServer

	logger log.Logger
	mu     sync.Mutex
	raft   Raft
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

func (svc *CompactionService) PollCompactionJobs(
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
	// index, and unsure isolation at the data level. For now, we're using
	// the raft log to guarantee serializable isolation level.
	//
	// Make sure that only one compaction plan update is in progress at a time.
	// This lock does not introduce contention, as the raft log is synchronous.
	svc.mu.Lock()
	defer svc.mu.Unlock()

	// First, we ask the current leader to prepare the change. This is a read
	// operation conducted through the raft log: at this stage, we only
	// prepare changes; the command handler does not alter the state.
	request := &raft_log.GetCompactionPlanUpdateRequest{
		StatusUpdates: make([]*raft_log.CompactionJobStatusUpdate, 0, len(req.StatusUpdates)),
		AssignJobsMax: req.JobCapacity,
	}

	// We only send the status updates (without job results) to minimize the
	// traffic, but we want to include the results of compaction in the final
	// proposal. If the status update is accepted, we trust the worker and
	// don't need to load our own copy of the job.
	compacted := make(map[string]*metastorev1.CompactionJobStatusUpdate, len(req.StatusUpdates))
	for _, update := range req.StatusUpdates {
		if update.CompactedBlocks != nil {
			compacted[update.Name] = update
		}
		request.StatusUpdates = append(request.StatusUpdates, &raft_log.CompactionJobStatusUpdate{
			Name:   update.Name,
			Token:  update.Token,
			Status: update.Status,
		})
	}

	cmd := fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_GET_COMPACTION_PLAN_UPDATE)
	resp, err := svc.raft.Propose(cmd, req)
	if err != nil {
		level.Error(svc.logger).Log("msg", "failed to prepare compaction plan", "err", err)
		return nil, err
	}
	prepared := resp.(*raft_log.GetCompactionPlanUpdateResponse)
	planUpdate := prepared.GetPlanUpdate()

	// Copy plan updates to the worker response. The job plan is only sent for
	// newly assigned jobs. Lease renewals do not require the plan to be sent.
	workerResp := &metastorev1.PollCompactionJobsResponse{
		CompactionJobs: make([]*metastorev1.CompactionJob, 0, len(planUpdate.AssignedJobs)),
		Assignments:    make([]*metastorev1.CompactionJobAssignment, 0, len(planUpdate.UpdatedJobs)),
	}
	for _, updated := range planUpdate.UpdatedJobs {
		update := updated.State
		workerResp.Assignments = append(workerResp.Assignments, &metastorev1.CompactionJobAssignment{
			Name:           update.Name,
			Token:          update.Token,
			LeaseExpiresAt: update.LeaseExpiresAt,
		})
	}
	for _, assigned := range planUpdate.AssignedJobs {
		assignment := assigned.State
		workerResp.Assignments = append(workerResp.Assignments, &metastorev1.CompactionJobAssignment{
			Name:           assignment.Name,
			Token:          assignment.Token,
			LeaseExpiresAt: assignment.LeaseExpiresAt,
		})
		job := assigned.Plan
		workerResp.CompactionJobs = append(workerResp.CompactionJobs, &metastorev1.CompactionJob{
			Name:            job.Name,
			Shard:           job.Shard,
			Tenant:          job.Tenant,
			CompactionLevel: job.CompactionLevel,
			SourceBlocks:    job.SourceBlocks,
			Tombstones:      job.Tombstones,
		})
		// Assigned jobs are not written to the raft log (only the assignments):
		// from our perspective (scheduler and planner) these are just job updates.
		assigned.Plan = nil
	}

	// Include the compacted blocks in the final proposal.
	for _, job := range planUpdate.CompletedJobs {
		if update := compacted[job.State.Name]; update != nil {
			job.CompactedBlocks = update.CompactedBlocks
		}
	}

	// Now that we have the plan, we need to propagate it through the
	// raft log to ensure it is applied consistently across all replicas,
	// regardless of their individual state or view of the plan.
	cmd = fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN)

	// We also include the current term of the planning step so that later
	// we can verify that the leader has not changed, and the plan is still
	// up-to-date. Otherwise, e.g., in the ABA case, when the current node
	// loses leadership and gains is back in-between these two steps, we
	// cannot guarantee that the proposed plan is still valid and up-to-date.
	// The raft handler cannot return an error here (because this is a valid
	// scenario, and we don't want to stop the node/cluster). Instead, an
	// empty response would indicate that the plan is rejected.
	proposal := &raft_log.UpdateCompactionPlanRequest{Term: prepared.Term, PlanUpdate: planUpdate}
	if resp, err = svc.raft.Propose(cmd, proposal); err != nil {
		level.Error(svc.logger).Log("msg", "failed to update compaction plan", "err", err)
		return nil, err
	}
	accepted := resp.(*raft_log.UpdateCompactionPlanResponse).GetPlanUpdate()
	if accepted == nil {
		level.Warn(svc.logger).Log("msg", "compaction plan update rejected")
		return nil, status.Error(codes.FailedPrecondition, "failed to update compaction plan")
	}

	// As of now, accepted plan always matches the proposed one,
	// so our prepared worker response is still valid.
	return workerResp, nil
}
