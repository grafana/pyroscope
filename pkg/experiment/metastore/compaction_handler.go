package metastore

import (
	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

type CompactionPlanner interface {
	NewPlan() compaction.Plan
}

type CompactionPlan interface {
	ApplyUpdate(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error
}

type CompactionCommandHandler struct {
	logger  log.Logger
	planner CompactionPlanner
	plan    CompactionPlan
}

func (h *CompactionCommandHandler) GetCompactionPlanUpdate(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.GetCompactionPlanUpdateRequest,
) (*raft_log.GetCompactionPlanUpdateResponse, error) {
	p := &raft_log.CompactionPlanUpdate{
		CompactionJobs: make([]*metastorev1.CompactionJob, 0, req.AssignJobsMax),
		JobUpdates:     make([]*raft_log.CompactionJobState, 0, len(req.StatusUpdates)),
	}

	plan := h.planner.NewPlan()

	// Request job status updates.
	for _, status := range req.StatusUpdates {
		p.JobUpdates = append(p.JobUpdates, plan.UpdateJob(tx, cmd, status))
	}

	// Request to assign new jobs.
	for len(p.CompactionJobs) < int(req.AssignJobsMax) {
		job, assignment := plan.AssignJob(tx, cmd)
		if job == nil {
			break
		}
		p.CompactionJobs = append(p.CompactionJobs, job)
		p.JobUpdates = append(p.JobUpdates, assignment)
	}

	// Request to create more jobs: we expect that at least
	// the requested job capacity is utilized next time we ask
	// for new assignments (this worker instance or not).
	for len(p.CompactionJobs) < int(req.AssignJobsMax) {
		job := plan.CreateJob(tx, cmd)
		if job == nil {
			break
		}
		p.CompactionJobs = append(p.CompactionJobs, job)
	}

	return &raft_log.GetCompactionPlanUpdateResponse{PlanUpdate: p}, nil
}

func (h *CompactionCommandHandler) UpdateCompactionPlan(
	tx *bbolt.Tx, _ *raft.Log, req *raft_log.UpdateCompactionPlanRequest,
) (*raft_log.UpdateCompactionPlanResponse, error) {
	if err := h.plan.ApplyUpdate(tx, req.PlanUpdate); err != nil {
		return nil, err
	}
	return new(raft_log.UpdateCompactionPlanResponse), nil
}
