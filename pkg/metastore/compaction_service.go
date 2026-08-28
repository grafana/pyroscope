package metastore

import (
	"context"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode"
)

// jobOwnerTTL limits how long the worker attribution of a job is kept
// after the last observed status update. Attribution of a job that is
// not updated (e.g., a failed job that exceeded the failure threshold)
// expires, and the job owner is reported as unknown.
const jobOwnerTTL = time.Hour

type CompactionService struct {
	metastorev1.CompactionServiceServer

	logger    log.Logger
	mu        sync.Mutex
	raft      Raft
	state     State
	scheduler *scheduler.Scheduler
	compactor *compactor.Compactor

	// Worker attribution of the assigned jobs, as observed by the local
	// node serving the poll requests (the raft leader). The attribution
	// is not replicated: after a leader change, it is restored gradually,
	// as workers renew their job leases. An entry is only valid while its
	// fencing token matches the job state.
	ownersMu sync.Mutex
	owners   map[string]*jobOwner
}

type jobOwner struct {
	worker     string
	token      uint64
	assignedAt time.Time
	updatedAt  time.Time
}

func NewCompactionService(
	logger log.Logger,
	raft Raft,
	state State,
	scheduler *scheduler.Scheduler,
	compactor *compactor.Compactor,
) *CompactionService {
	return &CompactionService{
		logger:    logger,
		raft:      raft,
		state:     state,
		scheduler: scheduler,
		compactor: compactor,
		owners:    make(map[string]*jobOwner),
	}
}

func (svc *CompactionService) PollCompactionJobs(
	ctx context.Context,
	req *metastorev1.PollCompactionJobsRequest,
) (resp *metastorev1.PollCompactionJobsResponse, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "CompactionService.PollCompactionJobs")
	defer func() {
		if err != nil {
			span.LogError(err)
			span.SetError()
		}
		span.Finish()
	}()

	span.SetTag("status_updates", len(req.GetStatusUpdates()))
	span.SetTag("job_capacity", req.GetJobCapacity())

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
	proposeReq := &raft_log.GetCompactionPlanUpdateRequest{
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
		proposeReq.StatusUpdates = append(proposeReq.StatusUpdates, &raft_log.CompactionJobStatusUpdate{
			Name:   update.Name,
			Token:  update.Token,
			Status: update.Status,
		})
	}

	cmd := fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_GET_COMPACTION_PLAN_UPDATE)
	proposeResp, err := svc.raft.Propose(ctx, cmd, proposeReq)
	if err != nil {
		if !raftnode.IsRaftLeadershipError(err) {
			level.Error(svc.logger).Log("msg", "failed to prepare compaction plan", "err", err)
		}
		return nil, err
	}
	prepared := proposeResp.(*raft_log.GetCompactionPlanUpdateResponse)
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
	if proposeResp, err = svc.raft.Propose(ctx, cmd, proposal); err != nil {
		if !raftnode.IsRaftLeadershipError(err) {
			level.Error(svc.logger).Log("msg", "failed to update compaction plan", "err", err)
		}
		return nil, err
	}
	accepted := proposeResp.(*raft_log.UpdateCompactionPlanResponse).GetPlanUpdate()
	if accepted == nil {
		level.Warn(svc.logger).Log("msg", "compaction plan update rejected")
		return nil, status.Error(codes.FailedPrecondition, "failed to update compaction plan")
	}

	// As of now, accepted plan always matches the proposed one, so our prepared
	// worker response is still valid.
	svc.updateOwners(workerID(ctx, req), accepted)

	span.SetTag("assigned_jobs", len(workerResp.GetCompactionJobs()))
	span.SetTag("assignment_updates", len(workerResp.GetAssignments()))
	return workerResp, nil
}

// workerID identifies the worker on a best-effort basis: the identity reported
// by the worker itself, or the peer address as a fallback.
func workerID(ctx context.Context, req *metastorev1.PollCompactionJobsRequest) string {
	if req.WorkerId != "" {
		return req.WorkerId
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return p.Addr.String()
	}
	return ""
}

func (svc *CompactionService) updateOwners(worker string, update *raft_log.CompactionPlanUpdate) {
	now := time.Now()
	svc.ownersMu.Lock()
	defer svc.ownersMu.Unlock()
	for _, job := range update.CompletedJobs {
		delete(svc.owners, job.State.Name)
	}
	for _, job := range update.EvictedJobs {
		delete(svc.owners, job.State.Name)
	}
	for _, job := range update.AssignedJobs {
		svc.owners[job.State.Name] = &jobOwner{
			worker:     worker,
			token:      job.State.Token,
			assignedAt: now,
			updatedAt:  now,
		}
	}
	for _, job := range update.UpdatedJobs {
		owner := svc.owners[job.State.Name]
		if owner == nil || owner.token != job.State.Token {
			// The job was assigned before we started observing it,
			// e.g., by a former leader: the assignment time is unknown.
			owner = &jobOwner{token: job.State.Token}
			svc.owners[job.State.Name] = owner
		}
		owner.worker = worker
		owner.updatedAt = now
	}
	for name, owner := range svc.owners {
		if now.Sub(owner.updatedAt) > jobOwnerTTL {
			delete(svc.owners, name)
		}
	}
}

func (svc *CompactionService) GetCompactionState(
	ctx context.Context,
	req *metastorev1.GetCompactionStateRequest,
) (resp *metastorev1.GetCompactionStateResponse, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "CompactionService.GetCompactionState")
	defer func() {
		if err != nil {
			span.LogError(err)
			span.SetError()
		}
		span.Finish()
	}()

	// The tenant filter distinguishes "every tenant" from "the entities that
	// have no tenant", so it is read from the field itself rather than from
	// the accessor: the field carries explicit presence.
	var tenantFilter *string
	if req != nil {
		tenantFilter = req.Tenant
	}
	jobFilter := scheduler.JobFilter{
		Tenant:              tenantFilter,
		IncludeSourceBlocks: req.GetIncludeSourceBlocks(),
	}
	queueFilter := compactor.QueueFilter{Tenant: tenantFilter}

	var jobs []scheduler.JobInfo
	var queues []compactor.QueueStats
	var readErr error
	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		if jobs, readErr = svc.scheduler.ListJobs(tx, jobFilter); readErr != nil {
			return
		}
		queues, readErr = svc.compactor.ListQueues(ctx, tx, queueFilter)
	}
	if err = svc.state.ConsistentRead(ctx, read); err != nil {
		// Preserve the status details, if any: e.g., the raft leader hint
		// attached at ReadIndex, which the client uses to redirect the
		// request to the leader node.
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	if readErr != nil {
		return nil, status.Error(codes.Internal, readErr.Error())
	}

	config := svc.scheduler.Config()
	resp = &metastorev1.GetCompactionStateResponse{
		CompactionJobs:   make([]*metastorev1.CompactionJobDetails, 0, len(jobs)),
		CompactionQueues: make([]*metastorev1.CompactionQueueDetails, 0, len(queues)),
		JobLeaseDuration: config.LeaseDuration.Nanoseconds(),
		JobMaxFailures:   config.MaxFailures,
		MaxJobQueueSize:  config.MaxQueueSize,
	}

	svc.ownersMu.Lock()
	for _, job := range jobs {
		details := &metastorev1.CompactionJobDetails{
			Name:            job.State.Name,
			Tenant:          job.Tenant,
			Shard:           job.Shard,
			CompactionLevel: job.State.CompactionLevel,
			Status:          job.State.Status,
			Token:           job.State.Token,
			LeaseExpiresAt:  job.State.LeaseExpiresAt,
			AddedAt:         job.State.AddedAt,
			Failures:        job.State.Failures,
			SourceBlocks:    job.SourceBlocks,
			SourceBlockIds:  job.SourceBlockIDs,
		}
		if owner := svc.owners[job.State.Name]; owner != nil && owner.token == job.State.Token {
			details.WorkerId = owner.worker
			details.UpdatedAt = owner.updatedAt.UnixNano()
			if !owner.assignedAt.IsZero() {
				details.AssignedAt = owner.assignedAt.UnixNano()
			}
		}
		resp.CompactionJobs = append(resp.CompactionJobs, details)
	}
	svc.ownersMu.Unlock()

	for _, q := range queues {
		resp.CompactionQueues = append(resp.CompactionQueues, &metastorev1.CompactionQueueDetails{
			Tenant:          q.Tenant,
			Shard:           q.Shard,
			CompactionLevel: q.Level,
			Blocks:          q.Blocks,
			OldestBlockAt:   q.OldestAppendedAt,
			NewestBlockAt:   q.NewestAppendedAt,
		})
	}

	span.SetTag("compaction_jobs", len(resp.CompactionJobs))
	span.SetTag("compaction_queues", len(resp.CompactionQueues))
	return resp, nil
}
