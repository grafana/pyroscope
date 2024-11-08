package compactor

import (
	"container/heap"
	"slices"
	"time"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

// schedule should be used to prepare the compaction plan update.
// The implementation must have no side effects or alter the
// Scheduler in any way.
type schedule struct {
	tx        *bbolt.Tx
	raft      *raft.Log
	scheduler *Scheduler
	assigner  *jobAssigner
}

func (p *schedule) AddJob(plan *raft_log.CompactionJobPlan) (*compaction.Job, error) {
	state := p.scheduler.queue.jobs[plan.Name]
	if state != nil {
		// Even if the job already exists, we will try to reset its state.
		// This should never happen; indicates a bug in the compaction planner.
	}

	job := &compaction.Job{
		Plan: plan,
		State: &raft_log.CompactionJobState{
			Name:            plan.Name,
			CompactionLevel: plan.CompactionLevel,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
			AddedAt:         p.raft.AppendedAt.UnixNano(),
			Token:           p.raft.Index,
		},
	}

	return job, nil
}

func (p *schedule) UpdateJob(update *metastorev1.CompactionJobStatusUpdate) (*compaction.Job, error) {
	state := p.scheduler.queue.jobs[update.Name]
	if state == nil {
		// This may happen if the job has been reassigned
		// and completed by another worker.
		return nil, nil
	}

	if state.Token > update.Token {
		// The job is not assigned to this worker.
		return nil, nil
	}

	switch newState := state.CloneVT(); update.Status {
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
		// A regular lease renewal.
		newState.LeaseExpiresAt = p.assigner.allocateLease()
		return &compaction.Job{State: newState}, nil

	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
		// The state does not change: it will be removed when the update
		// is applied. The contract requires scheduler to provide the completed
		// job plan with results to the planner.
		return p.completeJob(update)

	default:
		// Not allowed and unknown status updates can be safely ignored:
		// eventually, the job will be reassigned. The same for status
		// handlers: a nil state is returned, which is interpreted as
		// "no new lease, stop the work".
	}

	return nil, nil
}

func (p *schedule) completeJob(update *metastorev1.CompactionJobStatusUpdate) (*compaction.Job, error) {
	completed := update.CompactedBlocks
	if completed == nil {
		return nil, nil
	}

	job, err := p.loadJobPlan(update.Name)
	if err != nil || job == nil {
		return nil, err
	}
	job.DeletedBlocks = completed.DeletedBlocks
	job.CompactedBlocks = completed.CompactedBlocks

	return &compaction.Job{Plan: job}, nil
}

func (p *schedule) AssignJob() (*compaction.Job, error) {
	state := p.assigner.assign()
	if state == nil {
		return nil, nil
	}

	job, err := p.loadJobPlan(state.Name)
	if err != nil || job == nil {
		return nil, err
	}

	return &compaction.Job{State: state, Plan: job}, err
}

func (p *schedule) loadJobPlan(name string) (*raft_log.CompactionJobPlan, error) {
	job, err := p.scheduler.store.GetJobPlan(p.tx, name)
	if err != nil {
		return nil, err
	}
	if job == nil {
		// Job state exists without a plan. This should never happen.
		// If we keep the job in the queue (as it cannot be assigned),
		// it will be dangling there forever. Therefore, we remove it
		// now: this is an exceptional case no state should be changed
		// at scheduling.
		p.deleteDangling(name)
		return nil, nil
	}
	return job, nil
}

func (p *schedule) deleteDangling(name string) {
	_ = p.scheduler.store.DeleteJobState(p.tx, name)
	_ = p.scheduler.store.DeleteJobPlan(p.tx, name)
	p.assigner.queue.delete(name)
}

type jobAssigner struct {
	raft   *raft.Log
	config SchedulerConfig
	queue  *jobQueue
	copied []priorityQueue
	level  int
}

func (a *jobAssigner) assign() *raft_log.CompactionJobState {
	// We don't need to check the job ownership here: the worker asks
	// for a job assigment (new ownership).

	for a.level < len(a.queue.levels) {
		pq := a.queueLevelCopy(a.level)
		if pq.Len() == 0 {
			a.level++
			continue
		}

		switch job := heap.Pop(pq).(*jobEntry); job.Status {
		case metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED:
			return a.assignJob(job)

		case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
			if a.shouldReassign(job) {
				state := a.assignJob(job)
				state.Failures++
				return state
			}
		}

		// If no jobs can be assigned at this level,
		// we navigate to the next one.
		a.level++
	}

	return nil
}

func (a *jobAssigner) now() time.Time { return a.raft.AppendedAt }

func (a *jobAssigner) allocateLease() int64 { return a.now().Add(a.config.LeaseDuration).UnixNano() }

func (a *jobAssigner) assignJob(e *jobEntry) *raft_log.CompactionJobState {
	job := e.CompactionJobState.CloneVT()
	job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
	job.LeaseExpiresAt = a.allocateLease()
	job.Token = a.raft.Index
	return job
}

func (a *jobAssigner) shouldReassign(job *jobEntry) bool {
	abandoned := a.now().UnixNano() > job.LeaseExpiresAt
	faulty := a.config.MaxFailures > 0 && job.Failures > a.config.MaxFailures
	return abandoned && !faulty
}

// The queue must not be modified by assigner. Therefore, we're copying the
// queue levels lazily. The queue is supposed to be small (dozens of jobs
// running concurrently); in the worst case, we have a ~24b alloc per entry.
// Alternatively, we could push back the jobs to the queue, but it would
// require an explicit rollback call.
func (a *jobAssigner) queueLevelCopy(i int) *priorityQueue {
	s := i + 1 // Levels are 0-based.
	if s >= len(a.copied) || len(a.copied[i]) == 0 {
		a.copied = slices.Grow(a.copied, s)[:s]
		level := *a.queue.level(uint32(i))
		a.copied[i] = make(priorityQueue, len(level))
		for j, job := range level {
			jobCopy := *job
			a.copied[i][j] = &jobCopy
		}
	}
	return &a.copied[i]
}
