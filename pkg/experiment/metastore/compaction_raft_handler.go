package metastore

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

type IndexReplacer interface {
	ReplaceBlocks(*bbolt.Tx, *metastorev1.CompactedBlocks) error
}

type TombstoneDeleter interface {
	DeleteTombstones(*bbolt.Tx, *raft.Log, ...*metastorev1.Tombstones) error
	AddTombstones(*bbolt.Tx, *raft.Log, *metastorev1.Tombstones) error
}

type CompactionCommandHandler struct {
	logger     log.Logger
	index      IndexReplacer
	compactor  compaction.Compactor
	planner    compaction.Planner
	scheduler  compaction.Scheduler
	tombstones TombstoneDeleter
}

func NewCompactionCommandHandler(
	logger log.Logger,
	index IndexReplacer,
	compactor compaction.Compactor,
	planner compaction.Planner,
	scheduler compaction.Scheduler,
	tombstones TombstoneDeleter,
) *CompactionCommandHandler {
	return &CompactionCommandHandler{
		logger:     logger,
		index:      index,
		compactor:  compactor,
		planner:    planner,
		scheduler:  scheduler,
		tombstones: tombstones,
	}
}

func (h *CompactionCommandHandler) GetCompactionPlanUpdate(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.GetCompactionPlanUpdateRequest,
) (*raft_log.GetCompactionPlanUpdateResponse, error) {
	// We need to generate a plan of the update caused by the new status
	// report from the worker. The plan will be used to update the schedule
	// after the Raft consensus is reached.
	planner := h.planner.NewPlan(cmd)
	schedule := h.scheduler.NewSchedule(tx, cmd)
	p := new(raft_log.CompactionPlanUpdate)

	// Any status update may translate to either a job lease refresh, or a
	// completed job. Status update might be rejected, if the worker has
	// lost the job. We treat revoked jobs as vacant slots for new
	// assignments, therefore we try to update jobs' status first.
	var revoked int
	for _, status := range req.StatusUpdates {
		switch state := schedule.UpdateJob(status); {
		case state == nil:
			// Nil state indicates that the job has been abandoned and
			// reassigned, or the request is not valid. This may happen
			// from time to time, and we should just ignore such requests.
			revoked++

		case state.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
			p.CompletedJobs = append(p.CompletedJobs, &raft_log.CompletedCompactionJob{State: state})

		case state.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
			p.UpdatedJobs = append(p.UpdatedJobs, &raft_log.UpdatedCompactionJob{State: state})

		default:
			// Unknown statuses are ignored. From the worker perspective,
			// the job is re-assigned.
		}
	}

	// AssignJobsMax tells us how many free slots the worker has. We need to
	// account for the revoked jobs, as they are freeing the worker slots.
	capacity := int(req.AssignJobsMax) + revoked

	// Next, we need to create new jobs and assign existing
	//
	// NOTE(kolesnikovae): On one hand, if we assign first, we may violate the
	// SJF principle. If we plan new jobs first, it may cause starvation of
	// lower-priority jobs, when the compaction worker does not keep up with
	// the high-priority job influx. As of now, we assign jobs before creating
	// ones. If we change it, we need to make sure that the Schedule
	// implementation allows doing this.
	for assigned := 0; assigned < capacity; assigned++ {
		job, err := schedule.AssignJob()
		if err != nil {
			level.Error(h.logger).Log("msg", "failed to assign compaction job", "err", err)
			return nil, err
		}
		if job != nil {
			p.AssignedJobs = append(p.AssignedJobs, job)
		}
	}

	for created := 0; created < capacity; created++ {
		// Evict jobs that cannot be assigned to workers.
		if evicted := schedule.EvictJob(); evicted != nil {
			level.Debug(h.logger).Log("msg", "planning to evict failed job", "job", evicted.Name)
			p.EvictedJobs = append(p.EvictedJobs, &raft_log.EvictedCompactionJob{
				State: evicted,
			})
		}
		plan, err := planner.CreateJob()
		if err != nil {
			level.Error(h.logger).Log("msg", "failed to create compaction job", "err", err)
			return nil, err
		}
		if plan == nil {
			// No more jobs to create.
			break
		}
		state := schedule.AddJob(plan)
		if state == nil {
			// Scheduler declined the job. The only case when this may happen
			// is when the scheduler queue is full; theoretically, this should
			// not happen, because we evicted jobs before creating new ones.
			// However, if all the jobs are healthy, we may end up here.
			level.Warn(h.logger).Log("msg", "compaction job rejected by scheduler")
			break
		}
		p.NewJobs = append(p.NewJobs, &raft_log.NewCompactionJob{
			State: state,
			Plan:  plan,
		})
	}

	return &raft_log.GetCompactionPlanUpdateResponse{Term: cmd.Term, PlanUpdate: p}, nil
}

func (h *CompactionCommandHandler) UpdateCompactionPlan(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.UpdateCompactionPlanRequest,
) (*raft_log.UpdateCompactionPlanResponse, error) {
	if req.Term != cmd.Term || req.GetPlanUpdate() == nil {
		level.Warn(h.logger).Log(
			"msg", "rejecting compaction plan update",
			"current_term", cmd.Term,
			"request_term", req.Term,
		)
		return new(raft_log.UpdateCompactionPlanResponse), nil
	}

	if err := h.planner.UpdatePlan(tx, req.PlanUpdate); err != nil {
		level.Error(h.logger).Log("msg", "failed to update compaction planner", "err", err)
		return nil, err
	}

	if err := h.scheduler.UpdateSchedule(tx, req.PlanUpdate); err != nil {
		level.Error(h.logger).Log("msg", "failed to update compaction schedule", "err", err)
		return nil, err
	}

	for _, job := range req.PlanUpdate.NewJobs {
		if err := h.tombstones.DeleteTombstones(tx, cmd, job.Plan.Tombstones...); err != nil {
			level.Error(h.logger).Log("msg", "failed to delete tombstones", "err", err)
			return nil, err
		}
	}

	for _, job := range req.PlanUpdate.CompletedJobs {
		compacted := job.GetCompactedBlocks()
		if compacted == nil || compacted.SourceBlocks == nil || len(compacted.NewBlocks) == 0 {
			level.Warn(h.logger).Log("msg", "compacted blocks are missing; skipping", "job", job.State.Name)
			continue
		}
		if err := h.tombstones.AddTombstones(tx, cmd, blockTombstonesForCompletedJob(job)); err != nil {
			level.Error(h.logger).Log("msg", "failed to add tombstones", "err", err)
			return nil, err
		}
		for _, block := range compacted.NewBlocks {
			if err := h.compactor.Compact(tx, compaction.NewBlockEntry(cmd, block)); err != nil {
				level.Error(h.logger).Log("msg", "failed to compact block", "err", err)
				return nil, err
			}
		}
		if err := h.index.ReplaceBlocks(tx, compacted); err != nil {
			level.Error(h.logger).Log("msg", "failed to replace blocks", "err", err)
			return nil, err
		}
	}

	return &raft_log.UpdateCompactionPlanResponse{PlanUpdate: req.PlanUpdate}, nil
}

func blockTombstonesForCompletedJob(job *raft_log.CompletedCompactionJob) *metastorev1.Tombstones {
	source := job.CompactedBlocks.SourceBlocks
	return &metastorev1.Tombstones{
		Blocks: &metastorev1.BlockTombstones{
			Name:            job.State.Name,
			Shard:           source.Shard,
			Tenant:          source.Tenant,
			CompactionLevel: job.State.CompactionLevel,
			Blocks:          source.Blocks,
		},
	}
}
