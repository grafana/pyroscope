package metastore

import (
	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

type CompactionCommandHandler struct {
	logger    log.Logger
	planner   compaction.Planner
	scheduler compaction.Scheduler
}

func (h *CompactionCommandHandler) GetCompactionPlanUpdate(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.GetCompactionPlanUpdateRequest,
) (*raft_log.GetCompactionPlanUpdateResponse, error) {
	p := &raft_log.CompactionPlanUpdate{
		NewJobs:         make([]*raft_log.CompactionJobPlan, 0, req.AssignJobsMax),
		ScheduleUpdates: make([]*raft_log.CompactionJobState, 0, len(req.StatusUpdates)),
	}

	schedule := h.scheduler.NewSchedule(tx, cmd)

	// Try to assign new jobs first.
	for len(p.NewJobs) < int(req.AssignJobsMax) {
		job, assignment, err := schedule.AssignJob()
		if err != nil {
			return nil, err
		}
		if job == nil {
			break
		}
		p.NewJobs = append(p.NewJobs, job)
		p.ScheduleUpdates = append(p.ScheduleUpdates, assignment)
	}

	// Request job status updates.
	for _, status := range req.StatusUpdates {
		update, err := schedule.UpdateJob(status)
		if err != nil {
			return nil, err
		}
		if update != nil {
			p.ScheduleUpdates = append(p.ScheduleUpdates, update)
		}
	}

	// Request to create more jobs: we expect that at least
	// the requested job capacity is utilized next time we ask
	// for new assignments (this worker instance or not).
	plan := h.planner.NewPlan(tx)

	for len(p.NewJobs) < int(req.AssignJobsMax) {
		job, err := plan.CreateJob()
		if err != nil {
			return nil, err
		}
		if job == nil {
			break
		}
		p.NewJobs = append(p.NewJobs, job)
	}

	return &raft_log.GetCompactionPlanUpdateResponse{PlanUpdate: p}, nil
}

func (h *CompactionCommandHandler) UpdateCompactionPlan(
	tx *bbolt.Tx, _ *raft.Log, req *raft_log.UpdateCompactionPlanRequest,
) (*raft_log.UpdateCompactionPlanResponse, error) {
	if err := h.scheduler.UpdateSchedule(tx, h.planner, req.PlanUpdate); err != nil {
		return nil, err
	}
	return new(raft_log.UpdateCompactionPlanResponse), nil
}
