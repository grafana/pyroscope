package metastore

import (
	"container/heap"
	"slices"
	"sync"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

// A priority queue for compaction jobs. Jobs are prioritized by the compaction
// level, and the deadline time.
//
// The queue is supposed to be used by the compaction planner to schedule jobs.
//
// Compaction workers own jobs while they are in progress. Ownership handling is
// implemented using lease deadlines and fencing tokens:
// https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html

type jobQueue struct {
	// Although the queue is supposed to be used by a single planner
	// in a synchronous manner, we still need to protect the queue
	// from concurrent read accesses, such as stats collection, and
	// listing jobs for debug purposes.
	// This is a write-intensive path, so we use a regular mutex.
	mu     sync.Mutex
	jobs   map[string]*jobQueueEntry
	levels []priorityQueue
	lease  int64
}

// newJobQueue creates a new job queue with the given lease duration.
//
// Typically, callers should update jobs at the interval not exceeding
// the half of the lease duration.
func newJobQueue(lease int64) *jobQueue {
	pq := make(priorityQueue, 0)
	heap.Init(&pq)
	return &jobQueue{
		jobs:  make(map[string]*jobQueueEntry),
		lease: lease,
	}
}

type jobQueueEntry struct {
	// The index of the job in the heap.
	index int
	// The original proto message.
	*compactionpb.StoredCompactionJob
}

func (c *jobQueueEntry) less(x *jobQueueEntry) bool {
	if c.Status != x.Status {
		// Pick jobs in the "initial" (unspecified) state first.
		return c.Status < x.Status
	}
	if c.LeaseExpiresAt != x.LeaseExpiresAt {
		// Jobs with earlier deadlines should be at the top.
		return c.LeaseExpiresAt < x.LeaseExpiresAt
	}
	return c.Name < x.Name
}

func (q *jobQueue) level(x uint32) *priorityQueue {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
	}
	return &q.levels[x]
}

func (q *jobQueue) dequeue(now int64, raftLogIndex uint64) *compactionpb.StoredCompactionJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.levels {
		pq := q.levels[i]
		if len(pq) == 0 {
			continue
		}
		job := pq[0]
		if job.Status == compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS && now <= job.LeaseExpiresAt {
			// If the top job is in progress and not expired, stop checking further
			continue
		}
		if job.Status == compactionpb.CompactionStatus_COMPACTION_STATUS_CANCELLED {
			// if we've reached cancelled jobs in the queue we have no work left
			continue
		}
		// Remove the top job from the priority queue, update the status and lease and push it back.
		heap.Pop(&pq)
		job.LeaseExpiresAt = q.getNewDeadline(now)
		job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS
		// If job.status is "in progress", the ownership of the job is being revoked.
		job.RaftLogIndex = raftLogIndex
		heap.Push(&pq, job)
		return job.StoredCompactionJob
	}
	return nil
}

func (q *jobQueue) update(name string, now int64, raftLogIndex uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
		job.LeaseExpiresAt = q.getNewDeadline(now)
		job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS
		// De-prioritize the job, as the deadline has been postponed.
		heap.Fix(q.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (q *jobQueue) cancel(name string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_CANCELLED
		heap.Fix(q.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (q *jobQueue) getNewDeadline(now int64) int64 {
	return now + q.lease
}

func (q *jobQueue) isOwner(name string, raftLogIndex uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
	}
	return true
}

func (q *jobQueue) evict(name string, raftLogIndex uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
		delete(q.jobs, name)
		heap.Remove(q.level(job.CompactionLevel), job.index)
	}
	return true
}

func (q *jobQueue) enqueue(job *compactionpb.StoredCompactionJob) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.jobs[job.Name]; exists {
		return false
	}
	j := &jobQueueEntry{StoredCompactionJob: job}
	q.jobs[job.Name] = j
	heap.Push(q.level(job.CompactionLevel), j)
	return true
}

func (q *jobQueue) release(name string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED
		job.RaftLogIndex = 0
		job.LeaseExpiresAt = 0
		heap.Fix(q.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (q *jobQueue) reset() {
	clear(q.levels)
	clear(q.jobs)
}

// TODO(kolesnikovae): container/heap is not very efficient,
//  consider implementing own heap, specific to the case.

type priorityQueue []*jobQueueEntry

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool { return pq[i].less(pq[j]) }

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	job := x.(*jobQueueEntry)
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
