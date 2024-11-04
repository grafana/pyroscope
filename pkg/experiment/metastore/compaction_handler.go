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
	ApplyUpdate(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error
}

type CompactionCommandHandler struct {
	logger  log.Logger
	planner CompactionPlanner
}

func (h *CompactionCommandHandler) GetCompactionPlanUpdate(
	_ *bbolt.Tx, cmd *raft.Log, req *raft_log.GetCompactionPlanUpdateRequest,
) (*raft_log.GetCompactionPlanUpdateResponse, error) {
	p := &raft_log.CompactionPlanUpdate{
		CompactionJobs: make([]*metastorev1.CompactionJob, 0, req.AssignJobsMax),
		JobUpdates:     make([]*raft_log.CompactionJobState, 0, len(req.StatusUpdates)),
	}

	c := h.planner.NewPlan()
	for len(p.CompactionJobs) < int(req.AssignJobsMax) {
		if job := c.GetCompactionJob(cmd); job != nil {
			p.CompactionJobs = append(p.CompactionJobs, job)
			p.JobUpdates = append(p.JobUpdates, c.AssignJob(cmd, job))
		}
		break
	}
	for _, status := range req.StatusUpdates {
		p.JobUpdates = append(p.JobUpdates, c.UpdateJob(cmd, status))
	}

	return &raft_log.GetCompactionPlanUpdateResponse{PlanUpdate: p}, nil
}

func (h *CompactionCommandHandler) UpdateCompactionPlan(
	tx *bbolt.Tx, _ *raft.Log, req *raft_log.UpdateCompactionPlanRequest,
) (*raft_log.UpdateCompactionPlanResponse, error) {
	if err := h.planner.ApplyUpdate(tx, req.PlanUpdate); err != nil {
		return nil, err
	}
	return new(raft_log.UpdateCompactionPlanResponse), nil
}
