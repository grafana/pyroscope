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
	mu   sync.Mutex
	jobs map[string]*jobQueueEntry
	pq   priorityQueue

	lease int64
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
		pq:    pq,
		lease: lease,
	}
}

type jobQueueEntry struct {
	// The index of the job in the heap.
	index int
	// The original proto message.
	*compactionpb.CompactionJob
}

func (c *jobQueueEntry) less(x *jobQueueEntry) bool {
	if c.Status != x.Status {
		// Pick jobs in the "initial" (unspecified) state first.
		return c.Status < x.Status
	}
	if c.CompactionLevel != x.CompactionLevel {
		// Compact lower level jobs first.
		return c.CompactionLevel < x.CompactionLevel
	}
	if c.LeaseExpiresAt != x.LeaseExpiresAt {
		// Jobs with earlier deadlines should be at the top.
		return c.LeaseExpiresAt < x.LeaseExpiresAt
	}

	return c.Name < x.Name
}

func (q *jobQueue) dequeue(now int64, raftLogIndex uint64) *compactionpb.CompactionJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.pq.Len() > 0 {
		job := q.pq[0]
		if job.Status == compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS &&
			now <= job.LeaseExpiresAt {
			// If the top job is in progress and not expired, stop checking further
			return nil
		}
		if job.Status == compactionpb.CompactionStatus_COMPACTION_STATUS_CANCELLED {
			// if we've reached cancelled jobs in the queue we have no work left
			return nil
		}
		// Actually remove it from the heap, update and push it back.
		heap.Pop(&q.pq)
		job.LeaseExpiresAt = q.getNewDeadline(now)
		job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS
		// If job.status is "in progress", the ownership of the job is being revoked.
		job.RaftLogIndex = raftLogIndex
		heap.Push(&q.pq, job)
		return job.CompactionJob
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
		heap.Fix(&q.pq, job.index)
		return true
	}
	return false
}

func (q *jobQueue) cancel(name string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_CANCELLED
		heap.Fix(&q.pq, job.index)
	}
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
		heap.Remove(&q.pq, job.index)
	}
	return true
}

func (q *jobQueue) enqueue(job *compactionpb.CompactionJob) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.jobs[job.Name]; exists {
		return false
	}
	j := &jobQueueEntry{CompactionJob: job}
	q.jobs[job.Name] = j
	heap.Push(&q.pq, j)
	return true
}

func (q *jobQueue) putJob(job *compactionpb.CompactionJob) {
	q.jobs[job.Name] = &jobQueueEntry{CompactionJob: job}
}

func (q *jobQueue) rebuild() {
	q.pq = slices.Grow(q.pq[0:], len(q.jobs))
	for _, job := range q.jobs {
		q.pq = append(q.pq, job)
	}
	heap.Init(&q.pq)
}

func (q *jobQueue) stats() (int, []string, []string, []string, []string, []string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	newJobs := make([]string, 0)
	inProgressJobs := make([]string, 0)
	completedJobs := make([]string, 0)
	failedJobs := make([]string, 0)
	cancelledJobs := make([]string, 0)
	for _, job := range q.jobs {
		switch job.Status {
		case compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED:
			newJobs = append(newJobs, job.Name)
		case compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
			inProgressJobs = append(inProgressJobs, job.Name)
		case compactionpb.CompactionStatus_COMPACTION_STATUS_SUCCESS:
			completedJobs = append(completedJobs, job.Name)
		case compactionpb.CompactionStatus_COMPACTION_STATUS_FAILURE:
			failedJobs = append(failedJobs, job.Name)
		case compactionpb.CompactionStatus_COMPACTION_STATUS_CANCELLED:
			cancelledJobs = append(cancelledJobs, job.Name)
		}
	}
	return len(q.jobs), newJobs, inProgressJobs, completedJobs, failedJobs, cancelledJobs
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
