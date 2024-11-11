package metastore

import (
	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

type IndexReplacer interface {
	ReplaceBlocks(*bbolt.Tx, *raft.Log, *metastorev1.CompactedBlocks) error
}

type CompactionCommandHandler struct {
	logger    log.Logger
	planner   compaction.Planner
	scheduler compaction.Scheduler
	index     IndexReplacer
}

func (h *CompactionCommandHandler) GetCompactionPlanUpdate(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.GetCompactionPlanUpdateRequest,
) (*raft_log.GetCompactionPlanUpdateResponse, error) {
	// We need to generate a plan of the update caused by the new status
	// report from the worker. The plan will be used to update the schedule
	// after the Raft consensus is reached.
	plan := h.planner.NewPlan(tx, cmd)
	schedule := h.scheduler.NewSchedule(tx, cmd)

	p := &raft_log.CompactionPlanUpdate{
		NewJobs:       make([]*raft_log.CompactionJobUpdate, 0, req.AssignJobsMax),
		AssignedJobs:  make([]*raft_log.CompactionJobUpdate, 0, len(req.StatusUpdates)),
		CompletedJobs: make([]*raft_log.CompactionJobUpdate, 0, len(req.StatusUpdates)),
	}

	// Any status update may translate to either a job lease refresh, or a
	// completed job. Status update might be rejected, if the worker has
	// lost the job. We treat revoked jobs as vacant slots for new
	// assignments, therefore we try to update jobs' status first.
	var revoked int
	for _, status := range req.StatusUpdates {
		job, err := schedule.UpdateJob(status)
		if err != nil {
			return nil, err
		}

		switch {
		case job == nil:
			// Nil update indicates that the job has been abandoned and
			// reassigned, or the request is not valid. This may happen
			// from time to time, and we should just ignore such requests.
			revoked++

		case job.State == nil:
			// Nil state indicates that the job has been completed.
			p.CompletedJobs = append(p.CompletedJobs, job)

		default:
			// A state has been updated. As of now, this is always a regular
			// job lease (assignment) refresh, which we need to propagate to
			// the worker.
			p.AssignedJobs = append(p.AssignedJobs, job)
		}
	}

	// AssignJobsMax tells us how many free slots the worker has. We need to
	// account for the revoked jobs, as they are freeing the worker slots.
	capacity := int(req.AssignJobsMax) + revoked

	// NOTE(kolesnikovae): Next, we need to create new jobs and assign existing
	// ones; On one hand, if we assign first, we may violate the SJF principle.
	// If we plan new jobs first, it may cause starvation of lower-priority
	// jobs, when the compaction worker does not keep up with the high-priority
	// job influx.
	//
	// As of now, we assign jobs before creating ones. If we change it, we need
	// to make sure that the Schedule implementation allows doing this.
	for assigned := 0; assigned < capacity; assigned++ {
		job, err := schedule.AssignJob()
		if err != nil {
			return nil, err
		}
		if job == nil {
			// No more jobs to assign.
			break
		}
		p.AssignedJobs = append(p.AssignedJobs, job)
	}

	for created := 0; created < capacity; created++ {
		planned, err := plan.CreateJob()
		if err != nil {
			return nil, err
		}
		if planned == nil {
			// No more jobs to create.
			break
		}
		newJob, err := schedule.AddJob(planned)
		if err != nil {
			return nil, err
		}
		p.NewJobs = append(p.NewJobs, newJob)
	}

	return &raft_log.GetCompactionPlanUpdateResponse{PlanUpdate: p}, nil
}

func (h *CompactionCommandHandler) UpdateCompactionPlan(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.UpdateCompactionPlanRequest,
) (*raft_log.UpdateCompactionPlanResponse, error) {
	if err := h.planner.UpdatePlan(tx, cmd, req.PlanUpdate); err != nil {
		return nil, err
	}
	if err := h.scheduler.UpdateSchedule(tx, cmd, req.PlanUpdate); err != nil {
		return nil, err
	}
	for _, job := range req.PlanUpdate.CompletedJobs {
		compacted := &metastorev1.CompactedBlocks{
			SourceBlocks:    job.Plan.SourceBlocks,
			CompactedBlocks: job.Plan.CompactedBlocks,
		}
		if err := h.index.ReplaceBlocks(tx, cmd, compacted); err != nil {
			return nil, err
		}
	}
	return new(raft_log.UpdateCompactionPlanResponse), nil
}
