package compactor

import (
	"container/heap"
	"slices"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type jobQueue struct {
	jobs   map[string]*jobEntry
	levels []priorityQueue
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

func newJobQueue() *jobQueue {
	return &jobQueue{jobs: make(map[string]*jobEntry)}
}

func (q *jobQueue) reset() {
	clear(q.levels)
	clear(q.jobs)
}

func (q *jobQueue) level(x uint32) *priorityQueue {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
	}
	return &q.levels[x]
}

func (q *jobQueue) put(state *raft_log.CompactionJobState) {
	job, exists := q.jobs[state.Name]
	if exists {
		job.CompactionJobState = state
		heap.Fix(q.level(state.CompactionLevel), job.index)
		return
	}
	j := &jobEntry{CompactionJobState: state}
	q.jobs[job.Name] = j
	heap.Push(q.level(state.CompactionLevel), j)
}

func (q *jobQueue) delete(job *raft_log.CompactionJobState) {
	if j, exists := q.jobs[job.Name]; exists {
		heap.Remove(q.level(job.CompactionLevel), q.jobs[j.Name].index)
	}
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
