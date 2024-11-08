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
	p := &raft_log.CompactionPlanUpdate{
		NewJobs:         make([]*raft_log.CompactionJobPlan, 0, req.AssignJobsMax),
		ScheduleUpdates: make([]*raft_log.CompactionJobState, 0, len(req.StatusUpdates)),
	}

	schedule := h.scheduler.NewSchedule(tx, cmd)
	for _, status := range req.StatusUpdates {
		job, err := schedule.UpdateJob(status)
		if err != nil {
			return nil, err
		}
		if job == nil {
			// Nil indicates that the job has been abandoned and reassigned,
			// or the request is not valid. This may happen from time to time,
			// and we should just ignore such requests.
			continue
		}
		// There are two possible outcomes: the job is completed, or its state
		// has been updated. If state is not present, the job is completed, and
		// the results are added to the job. Otherwise, this is an update, and
		// the job plan is not updated (= not present).
		if job.State == nil {
			p.CompletedJobs = append(p.CompletedJobs, job.Plan)
		} else {
			p.ScheduleUpdates = append(p.ScheduleUpdates, job.State)
		}
	}

	// Try to assign existing jobs first. There are several reasons, the main
	// one being to keep the scheduler job queue short. Otherwise, new jobs may
	// be created and left in the queue unassigned for a long time if workers
	// do not have enough capacity to process them promptly.
	//
	// The worker may be left without new assignments, if the queue is empty
	// at the moment. If the data flow is stable, this does not increase the
	// job wait time any significantly: if new jobs are created due to our
	// update (added compacted blocks), they cannot be smaller (have higher
	// priority) than ones in the queue.
	//
	// However, this helps to prevent the case when the same worker handles
	// a chain of jobs on its own, when no new blocks arrive, and no new jobs
	// are created. Instead, we want others to pick the job to distribute the
	// load more evenly.
	for len(p.NewJobs) < int(req.AssignJobsMax) {
		job, err := schedule.AssignJob()
		if err != nil {
			return nil, err
		}
		if job == nil {
			// No more jobs to assign.
			break
		}
		p.ScheduleUpdates = append(p.ScheduleUpdates, job.State)
		p.AssignedJobs = append(p.AssignedJobs, job.Plan)
	}

	// Request to create more jobs: we expect that at least the requested job
	// capacity is utilized next time we ask for new assignments (this worker
	// instance or not).
	plan := h.planner.NewPlan(tx, cmd)
	for len(p.NewJobs) < int(req.AssignJobsMax) {
		planned, err := plan.CreateJob()
		if err != nil {
			return nil, err
		}
		if planned == nil {
			// No more jobs to create.
			break
		}
		job, err := schedule.AddJob(planned)
		if err != nil {
			return nil, err
		}
		p.NewJobs = append(p.NewJobs, planned)
		p.ScheduleUpdates = append(p.ScheduleUpdates, job.State)
	}

	return &raft_log.GetCompactionPlanUpdateResponse{PlanUpdate: p}, nil
}

func (h *CompactionCommandHandler) UpdateCompactionPlan(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.UpdateCompactionPlanRequest,
) (*raft_log.UpdateCompactionPlanResponse, error) {
	if err := h.planner.Scheduled(tx, req.PlanUpdate.NewJobs...); err != nil {
		return nil, err
	}

	for _, job := range req.PlanUpdate.CompletedJobs {
		compacted := &metastorev1.CompactedBlocks{
			CompactedBlocks: job.CompactedBlocks,
			DeletedBlocks:   job.DeletedBlocks,
		}
		if err := h.index.ReplaceBlocks(tx, cmd, compacted); err != nil {
			return nil, err
		}
	}

	if err := h.scheduler.UpdateSchedule(tx, req.PlanUpdate); err != nil {
		return nil, err
	}

	return new(raft_log.UpdateCompactionPlanResponse), nil
}
