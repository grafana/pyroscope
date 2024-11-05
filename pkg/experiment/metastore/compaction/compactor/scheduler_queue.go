package compactor

import (
	"container/heap"
	"slices"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type jobQueue struct {
	jobs   map[string]*jobEntry
	levels []priorityQueue
	lease  int64
}

type jobEntry struct {
	index int // The index of the job in the heap.
	*raft_log.CompactionJobState
}

func (c *jobEntry) less(x *jobEntry) bool {
	if c.Status != x.Status {
		// Pick jobs in the "initial" (unspecified) state first:
		// COMPACTION_STATUS_UNSPECIFIED <- Unassigned jobs.
		// COMPACTION_STATUS_IN_PROGRESS <- Assigned jobs.
		// COMPACTION_STATUS_SUCCESS <- Should never be in the queue.
		// COMPACTION_STATUS_FAILURE <- Should never be in the queue.
		// COMPACTION_STATUS_CANCELLED <- Unassigned jobs that should not be scheduled.
		return c.Status < x.Status
	}
	if c.LeaseExpiresAt != x.LeaseExpiresAt {
		// Jobs with earlier deadlines should be at the top.
		return c.LeaseExpiresAt < x.LeaseExpiresAt
	}
	return c.Name < x.Name
}

func newJobQueue(lease int64) *jobQueue {
	return &jobQueue{
		jobs:  make(map[string]*jobEntry),
		lease: lease,
	}
}

func (q *jobQueue) level(x uint32) *priorityQueue {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
	}
	return &q.levels[x]
}

func (q *jobQueue) put(job *raft_log.CompactionJobState) {
	j := &jobEntry{CompactionJobState: job}
	q.jobs[job.Name] = j
	heap.Push(q.level(job.CompactionLevel), j)
}

func (q *jobQueue) delete(job *raft_log.CompactionJobState) {
	if j, exists := q.jobs[job.Name]; exists {
		heap.Remove(q.level(job.CompactionLevel), q.jobs[j.Name].index)
	}
}

func (q *jobQueue) enqueue(job *raft_log.CompactionJobState) bool {
	if _, exists := q.jobs[job.Name]; exists {
		return false
	}
	j := &jobEntry{CompactionJobState: job}
	q.jobs[job.Name] = j
	heap.Push(q.level(job.CompactionLevel), j)
	return true
}

func (q *jobQueue) dequeue(now int64, raftLogIndex uint64) *raft_log.CompactionJobState {
	for i := range q.levels {
		pq := q.levels[i]
		if len(pq) == 0 {
			continue
		}
		job := pq[0]
		if job.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS && now <= job.LeaseExpiresAt {
			// If the top job is in progress and not expired, stop checking further
			continue
		}
		if job.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_CANCELLED {
			// if we've reached cancelled jobs in the queue we have no work left
			continue
		}
		// Remove the top job from the priority queue, update the status and lease and push it back.
		heap.Pop(&pq)
		job.LeaseExpiresAt = q.getNewDeadline(now)
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
		// If job.status is "in progress", the ownership of the job is being revoked.
		job.RaftLogIndex = raftLogIndex
		heap.Push(&pq, job)
		return job.CompactionJobState
	}
	return nil
}

func (q *jobQueue) update(name string, now int64, raftLogIndex uint64) bool {
	if job, exists := q.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
		job.LeaseExpiresAt = q.getNewDeadline(now)
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
		// De-prioritize the job, as the deadline has been postponed.
		heap.Fix(q.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (q *jobQueue) cancel(name string) bool {
	if job, exists := q.jobs[name]; exists {
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_CANCELLED
		heap.Fix(q.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (q *jobQueue) evict(name string, raftLogIndex uint64) bool {
	if job, exists := q.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
		delete(q.jobs, name)
		heap.Remove(q.level(job.CompactionLevel), job.index)
	}
	return true
}

func (q *jobQueue) release(name string) bool {
	if job, exists := q.jobs[name]; exists {
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED
		job.RaftLogIndex = 0
		job.LeaseExpiresAt = 0
		heap.Fix(q.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (q *jobQueue) getNewDeadline(now int64) int64 {
	return now + q.lease
}

func (q *jobQueue) isOwner(name string, raftLogIndex uint64) bool {
	if job, exists := q.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
	}
	return true
}

func (q *jobQueue) reset() {
	clear(q.levels)
	clear(q.jobs)
}

// TODO(kolesnikovae): container/heap is not very efficient,
//  consider implementing own heap, specific to the case.

type priorityQueue []*jobEntry

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool { return pq[i].less(pq[j]) }

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	job := x.(*jobEntry)
	job.index = n
	*pq = append(*pq, job)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	job := old[n-1]
	old[n-1] = nil
	job.index = -1
	*pq = old[0 : n-1]
	return job
}
