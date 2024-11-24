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
	updates map[string]*raft_log.CompactionJobState
	// Modified copy of the job queue.
	copied []priorityJobQueue
	level  int
}

func (p *schedule) AssignJob() (*raft_log.AssignedCompactionJob, error) {
	state := p.nextAssignment()
	if state == nil {
		return nil, nil
	}
	plan, err := p.scheduler.store.GetJobPlan(p.tx, state.Name)
	if err != nil {
		return nil, err
	}
	p.updates[state.Name] = state
	assigned := &raft_log.AssignedCompactionJob{
		State: state,
		Plan:  plan,
	}
	return assigned, nil
}

func (p *schedule) UpdateJob(status *raft_log.CompactionJobStatusUpdate) *raft_log.CompactionJobState {
	state := p.newStateForStatusReport(status)
	if state == nil {
		return nil
	}
	// State changes should be taken into account when we assign jobs.
	p.updates[status.Name] = state
	return state
}

// handleStatusReport reports the job state change caused by the status report
// from compaction worker. The function does not modify the actual job queue.
func (p *schedule) newStateForStatusReport(status *raft_log.CompactionJobStatusUpdate) *raft_log.CompactionJobState {
	state := p.scheduler.queue.jobs[status.Name]
	if state == nil {
		// This may happen if the job has been reassigned
		// and completed by another worker; we respond in
		// the same way.
		return nil
	}

	if state.Token > status.Token {
		// The job is not assigned to this worker.
		return nil
	}

	switch newState := state.CloneVT(); status.Status {
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
		// A regular lease renewal.
		newState.LeaseExpiresAt = p.allocateLease()
		return newState

	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
		newState.Status = status.Status
		return newState

	default:
		// Not allowed and unknown status updates can be safely ignored:
		// eventually, the job will be reassigned. The same for status
		// handlers: a nil state is returned, which is interpreted as
		// "no new lease, stop the work".
	}

	return nil
}

// AddJob creates a state for the new plan. The method must be called
// after the last AssignJob and UpdateJob calls.
func (p *schedule) AddJob(plan *raft_log.CompactionJobPlan) *raft_log.CompactionJobState {
	// TODO(kolesnikovae): Job queue size limit.
	state := &raft_log.CompactionJobState{
		Name:            plan.Name,
		CompactionLevel: plan.CompactionLevel,
		Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
		AddedAt:         p.now.UnixNano(),
		Token:           p.token,
	}
	p.updates[state.Name] = state
	return state
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
		if p.copied[i] == nil {
			p.copied[i] = p.scheduler.queue.level(uint32(i)).clone()
		}
	}
	return &p.copied[i]
}
