package compaction

import (
	"container/heap"
	"slices"
	"sync"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

// Compaction job scheduler. Jobs are prioritized by the compaction level, and
// the deadline time.
//
// Compaction workers own jobs while they are in progress. Ownership handling is
// implemented using lease deadlines and fencing tokens:
// https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html

type scheduler struct {
	// Although the scheduler is supposed to be used by a single planner
	// in a synchronous manner, we still need to protect it from concurrent
	// read accesses, such as stats collection, and listing jobs for debug
	// purposes. This is a write-intensive path, so we use a regular mutex.
	mu     sync.Mutex
	jobs   map[string]*jobEntry
	levels []priorityQueue
	lease  int64
}

// newScheduler creates a scheduler with the given lease duration.
// Typically, callers should update jobs at the interval not exceeding
// the half of the lease duration.
func newScheduler(lease int64) *scheduler {
	pq := make(priorityQueue, 0)
	heap.Init(&pq)
	return &scheduler{
		jobs:  make(map[string]*jobEntry),
		lease: lease,
	}
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

func (sc *scheduler) level(x uint32) *priorityQueue {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(sc.levels)) {
		sc.levels = slices.Grow(sc.levels, int(s))[:s]
	}
	return &sc.levels[x]
}

func (sc *scheduler) enqueue(job *raft_log.CompactionJobState) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if _, exists := sc.jobs[job.Name]; exists {
		return false
	}
	j := &jobEntry{CompactionJobState: job}
	sc.jobs[job.Name] = j
	heap.Push(sc.level(job.CompactionLevel), j)
	return true
}

func (sc *scheduler) dequeue(now int64, raftLogIndex uint64) *raft_log.CompactionJobState {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for i := range sc.levels {
		pq := sc.levels[i]
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
		job.LeaseExpiresAt = sc.getNewDeadline(now)
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
		// If job.status is "in progress", the ownership of the job is being revoked.
		job.RaftLogIndex = raftLogIndex
		heap.Push(&pq, job)
		return job.CompactionJobState
	}
	return nil
}

func (sc *scheduler) update(name string, now int64, raftLogIndex uint64) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if job, exists := sc.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
		job.LeaseExpiresAt = sc.getNewDeadline(now)
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
		// De-prioritize the job, as the deadline has been postponed.
		heap.Fix(sc.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (sc *scheduler) cancel(name string) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if job, exists := sc.jobs[name]; exists {
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_CANCELLED
		heap.Fix(sc.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (sc *scheduler) evict(name string, raftLogIndex uint64) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if job, exists := sc.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
		delete(sc.jobs, name)
		heap.Remove(sc.level(job.CompactionLevel), job.index)
	}
	return true
}

func (sc *scheduler) release(name string) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if job, exists := sc.jobs[name]; exists {
		job.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED
		job.RaftLogIndex = 0
		job.LeaseExpiresAt = 0
		heap.Fix(sc.level(job.CompactionLevel), job.index)
		return true
	}
	return false
}

func (sc *scheduler) getNewDeadline(now int64) int64 {
	return now + sc.lease
}

func (sc *scheduler) isOwner(name string, raftLogIndex uint64) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if job, exists := sc.jobs[name]; exists {
		if job.RaftLogIndex > raftLogIndex {
			return false
		}
	}
	return true
}

func (sc *scheduler) reset() {
	clear(sc.levels)
	clear(sc.jobs)
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
