package metastore

import (
	"container/heap"
	"slices"
	"sync"

	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
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

type jobStatus int

const (
	_ jobStatus = iota
	jobStatusInitial
	jobStatusInProgress
)

type jobQueueEntry struct {
	status jobStatus
	index  int // The index of the job in the heap.

	// The original proto message.
	proto *compactionpb.CompactionJob
}

func (c *jobQueueEntry) less(x *jobQueueEntry) bool {
	if c.status != x.status {
		// Peek jobs in the "initial" state first.
		return c.status < x.status
	}
	if c.proto.LeaseExpiresAt != x.proto.LeaseExpiresAt {
		// Jobs with earlier deadlines should be at the top.
		return c.proto.LeaseExpiresAt < x.proto.LeaseExpiresAt
	}
	// Compact lower level jobs first.
	if c.proto.CompactionLevel != x.proto.CompactionLevel {
		// Jobs with earlier deadlines should be at the top.
		return c.proto.CompactionLevel < x.proto.CompactionLevel
	}
	return c.proto.Name < x.proto.Name
}

func (c *jobQueueEntry) load(job *compactionpb.CompactionJob) {
	js := jobStatusInitial
	if job.Status == compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS {
		js = jobStatusInProgress
	}
	*c = jobQueueEntry{
		status: js,
		index:  0,
		proto:  job,
	}
	// Deadline will be assigned when the job is "dequeued" (transferred).
	job.RaftLogIndex = 0
	job.LeaseExpiresAt = 0
}

func (q *jobQueue) dequeue(now int64, raftLogIndex uint64) *compactionpb.CompactionJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.pq.Len() > 0 {
		job := q.pq[0]
		if job.status == jobStatusInProgress && now <= job.proto.LeaseExpiresAt {
			// If the top job is in progress and not expired, stop checking further
			return nil
		}
		// Actually remove it from the heap, update
		// and push it back.
		heap.Pop(&q.pq)
		job.proto.LeaseExpiresAt = q.getNewDeadline(now)
		job.status = jobStatusInProgress
		// if job.status is "in progress", the ownership of the job is being revoked.
		job.proto.RaftLogIndex = raftLogIndex
		heap.Push(&q.pq, job)
		return job.proto
	}
	return nil
}

func (q *jobQueue) update(name string, now int64, raftLogIndex uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		if job.proto.RaftLogIndex > raftLogIndex {
			return false
		}
		job.proto.LeaseExpiresAt = q.getNewDeadline(now)
		job.status = jobStatusInProgress
		// De-prioritize the job, as the deadline has been postponed.
		heap.Fix(&q.pq, job.index)
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
		if job.proto.RaftLogIndex > raftLogIndex {
			return false
		}
	}
	return true
}

func (q *jobQueue) evict(name string, raftLogIndex uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, exists := q.jobs[name]; exists {
		if job.proto.RaftLogIndex > raftLogIndex {
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
	var j jobQueueEntry
	j.load(job)
	q.jobs[job.Name] = &j
	heap.Push(&q.pq, &j)
	return true
}

func (q *jobQueue) putJob(job *compactionpb.CompactionJob) {
	var j jobQueueEntry
	j.load(job)
	q.jobs[job.Name] = &j
}

func (q *jobQueue) rebuild() {
	q.pq = slices.Grow(q.pq[0:], len(q.jobs))
	for _, job := range q.jobs {
		q.pq = append(q.pq, job)
	}
	heap.Init(&q.pq)
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
