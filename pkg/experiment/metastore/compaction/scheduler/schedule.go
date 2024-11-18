package scheduler

import (
	"container/heap"
	"slices"
	"time"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

// schedule should be used to prepare the compaction plan update.
// The implementation must have no side effects or alter the
// Scheduler in any way.
type schedule struct {
	tx    *bbolt.Tx
	now   time.Time
	token uint64
	// Read-only.
	scheduler *Scheduler
	// Uncommitted schedule updates.
	updates map[string]*raft_log.CompactionJobUpdate
	// Modified copy of the job queue.
	copied []priorityJobQueue
	level  int
}

func (p *schedule) AssignJob() (*raft_log.CompactionJobUpdate, error) {
	state := p.nextAssignment()
	if state == nil {
		return nil, nil
	}
	job, err := p.scheduler.store.GetJobPlan(p.tx, state.Name)
	if err != nil {
		return nil, err
	}
	update := &raft_log.CompactionJobUpdate{
		State: state,
		Plan:  job,
	}
	p.updates[state.Name] = update
	return update, err
}

func (p *schedule) UpdateJob(status *metastorev1.CompactionJobStatusUpdate) (*raft_log.CompactionJobUpdate, error) {
	state, err := p.newStateForStatusReport(status)
	if err != nil || state == nil {
		return nil, err
	}
	// State changes should be taken into account when we assign jobs.
	p.updates[status.Name] = state
	return state, nil
}

// handleStatusReport reports the job state change caused by the status report
// from compaction worker. The function does not modify the actual job queue.
func (p *schedule) newStateForStatusReport(status *metastorev1.CompactionJobStatusUpdate) (*raft_log.CompactionJobUpdate, error) {
	state := p.scheduler.queue.jobs[status.Name]
	if state == nil {
		// This may happen if the job has been reassigned
		// and completed by another worker.
		return nil, nil
	}

	if state.Token > status.Token {
		// The job is not assigned to this worker.
		return nil, nil
	}

	switch newState := state.CloneVT(); status.Status {
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
		// A regular lease renewal.
		newState.LeaseExpiresAt = p.allocateLease()
		return &raft_log.CompactionJobUpdate{State: newState}, nil

	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
		// The state does not change: it will be removed when the update
		// is applied. The contract requires scheduler to provide the completed
		// job plan with results to the planner.
		return p.completeJob(status)

	default:
		// Not allowed and unknown status updates can be safely ignored:
		// eventually, the job will be reassigned. The same for status
		// handlers: a nil state is returned, which is interpreted as
		// "no new lease, stop the work".
	}

	return nil, nil
}

func (p *schedule) completeJob(status *metastorev1.CompactionJobStatusUpdate) (*raft_log.CompactionJobUpdate, error) {
	job, err := p.scheduler.store.GetJobPlan(p.tx, status.Name)
	if err != nil {
		return nil, err
	}
	// We always trust the compaction job results more that the plan.
	// Theoretically, it's guaranteed that the stored list of source
	// blocks is identical to the one reported by the worker. However,
	// it's also guaranteed that the worker won't lie, and that these
	// are the blocks it processed.
	job.SourceBlocks = nil
	job.CompactedBlocks = status.CompactedBlocks
	// Tombstones are taken from the stored job plan.
	return &raft_log.CompactionJobUpdate{Plan: job}, nil
}

// AddJob creates a state for the new plan. The method must be called
// after the last AssignJob and UpdateJob calls.
func (p *schedule) AddJob(plan *raft_log.CompactionJobPlan) (*raft_log.CompactionJobUpdate, error) {
	// TODO(kolesnikovae): Job queue size limit.
	job := &raft_log.CompactionJobUpdate{
		Plan: plan,
		State: &raft_log.CompactionJobState{
			Name:            plan.Name,
			CompactionLevel: plan.CompactionLevel,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
			AddedAt:         p.now.UnixNano(),
			Token:           p.token,
		},
	}
	p.updates[job.Plan.Name] = job
	return job, nil
}

func (p *schedule) nextAssignment() *raft_log.CompactionJobState {
	// We don't need to check the job ownership here: the worker asks
	// for a job assigment (new ownership).
	for p.level < len(p.scheduler.queue.levels) {
		pq := p.queueLevelCopy(p.level)
		if pq.Len() == 0 {
			p.level++
			continue
		}

		job := heap.Pop(pq).(*jobEntry)
		if _, found := p.updates[job.Name]; found {
			// We don't even consider own jobs: these are already
			// assigned and are in-progress or have been completed.
			// This, however, does not prevent from reassigning a
			// job that the worker has abandoned in the past.
			// Newly created jobs are not considered here as well.
			continue
		}

		switch job.Status {
		case metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED:
			return p.assignJob(job)

		case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
			if p.shouldReassign(job) {
				state := p.assignJob(job)
				state.Failures++
				return state
			}
		}

		// If no jobs can be assigned at this level.
		p.level++
	}

	return nil
}

func (p *schedule) allocateLease() int64 {
	return p.now.Add(p.scheduler.config.LeaseDuration).UnixNano()
}

func (p *schedule) assignJob(e *jobEntry) *raft_log.CompactionJobState {
	job := e.CompactionJobState.CloneVT()
	job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
	job.LeaseExpiresAt = p.allocateLease()
	job.Token = p.token
	return job
}

func (p *schedule) shouldReassign(job *jobEntry) bool {
	abandoned := p.now.UnixNano() > job.LeaseExpiresAt
	limit := p.scheduler.config.MaxFailures
	faulty := limit > 0 && uint64(job.Failures) >= limit
	return abandoned && !faulty
}

// The queue must not be modified by the assigner. Therefore, we're copying the
// queue levels lazily. The queue is supposed to be small (hundreds of jobs
// running concurrently); in the worst case, we have a ~24b alloc per entry.
func (p *schedule) queueLevelCopy(i int) *priorityJobQueue {
	s := i + 1 // Levels are 0-based.
	if s > len(p.copied) {
		p.copied = slices.Grow(p.copied, s)[:s]
		level := *p.scheduler.queue.level(uint32(i))
		p.copied[i] = make(priorityJobQueue, len(level))
		for j, job := range level {
			jobCopy := *job
			p.copied[i][j] = &jobCopy
		}
	}
	return &p.copied[i]
}
