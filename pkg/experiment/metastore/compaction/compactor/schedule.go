package compactor

import (
	"container/heap"
	"slices"
	"time"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type schedule struct {
	tx        *bbolt.Tx
	raft      *raft.Log
	scheduler *Scheduler
	assigner  *jobAssigner
}

func (p *schedule) UpdateJob(update *metastorev1.CompactionJobStatusUpdate) (*raft_log.CompactionJobState, error) {
	state, err := p.scheduler.store.GetJobState(p.tx, update.Name)
	if err != nil {
		return nil, err
	}
	if state == nil {
		// The job is not found. This may happen if the job has been
		// reassigned and completed by another worker.
		return nil, nil
	}
	if state.Token > update.Token {
		// The job is not assigned to this worker.
		return nil, nil
	}

	switch updated := state.CloneVT(); update.Status {
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
		updated.LeaseExpiresAt = p.assigner.allocateLease()
		return updated, nil

	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
		// This is a valid status update from the worker, and we don't
		// need to do anything.
		return updated, nil

	default:
		// Not allowed and unknown status updates can be safely ignored:
		// eventually, the job will be reassigned. The same for status
		// handlers: a nil state is returned, which is interpreted as
		// "no new lease, stop the work".
		return nil, nil
	}
}

func (p *schedule) AssignJob() (*raft_log.CompactionJobPlan, *raft_log.CompactionJobState, error) {
	state := p.assigner.assign()
	if state == nil {
		return nil, nil, nil
	}
	job, err := p.scheduler.store.GetJob(p.tx, state.Name)
	if err != nil {
		return nil, nil, err
	}
	if job == nil {
		// Job not found. This should never happen and likely indicates
		// a data inconsistency. If we keep the job in the queue (as it
		// cannot be assigned), it will be dangling there forever.
		// Therefore, we remove it now: this is an exceptional case –
		// no state should be changed in compactionSchedule.
		p.deleteDangling(state)
		return nil, nil, nil
	}
	return job, state, nil
}

func (p *schedule) deleteDangling(state *raft_log.CompactionJobState) {
	_ = p.scheduler.store.DeleteJobState(p.tx, state.Name)
	_ = p.scheduler.store.DeleteJob(p.tx, state.Name)
	p.assigner.queue.delete(state)
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
