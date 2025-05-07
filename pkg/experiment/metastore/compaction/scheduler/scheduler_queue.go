package scheduler

import (
	"container/heap"
	"slices"
	"strings"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type schedulerQueue struct {
	jobs map[string]*jobEntry
	// Sparse array of job queues, indexed by compaction level.
	levels []*jobQueue
}

func newJobQueue() *schedulerQueue {
	return &schedulerQueue{
		jobs: make(map[string]*jobEntry),
	}
}

func (q *schedulerQueue) reset() {
	clear(q.jobs)
	clear(q.levels)
	q.levels = q.levels[:0]
}

func (q *schedulerQueue) put(state *raft_log.CompactionJobState) {
	job, exists := q.jobs[state.Name]
	level := q.level(state.CompactionLevel)
	if exists {
		level.update(job, state)
		return
	}
	e := &jobEntry{CompactionJobState: state}
	q.jobs[state.Name] = e
	level.add(e)
}

func (q *schedulerQueue) delete(name string) *raft_log.CompactionJobState {
	if e, exists := q.jobs[name]; exists {
		delete(q.jobs, name)
		level := q.level(e.CompactionLevel)
		level.delete(e)
		level.stats.completedTotal++
		return e.CompactionJobState
	}
	return nil
}

// evict is identical to delete, but it updates the eviction stats.
func (q *schedulerQueue) evict(name string) {
	if e, exists := q.jobs[name]; exists {
		delete(q.jobs, name)
		level := q.level(e.CompactionLevel)
		level.delete(e)
		level.stats.evictedTotal++
	}
}

func (q *schedulerQueue) size() int {
	var size int
	for _, level := range q.levels {
		if level != nil {
			size += level.jobs.Len()
		}
	}
	return size
}

func (q *schedulerQueue) level(x uint32) *jobQueue {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
	}
	level := q.levels[x]
	if level == nil {
		level = &jobQueue{
			jobs:  new(priorityJobQueue),
			stats: new(queueStats),
		}
		q.levels[x] = level
	}
	return level
}

func (q *schedulerQueue) resetStats() {
	for _, level := range q.levels {
		if level != nil {
			level.stats.reset()
		}
	}
}

type jobQueue struct {
	jobs  *priorityJobQueue
	stats *queueStats
}

type queueStats struct {
	// Counters. Updated on access.
	addedTotal      uint32
	completedTotal  uint32
	assignedTotal   uint32
	reassignedTotal uint32
	evictedTotal    uint32
	// Gauges. Updated periodically.
	assigned   uint32
	unassigned uint32
	reassigned uint32
	failed     uint32
}

func (s *queueStats) reset() {
	*s = queueStats{}
}

type jobEntry struct {
	index int // The index of the job in the heap.
	*raft_log.CompactionJobState
}

func (q *jobQueue) add(e *jobEntry) {
	q.stats.addedTotal++
	heap.Push(q.jobs, e)
}

func (q *jobQueue) update(e *jobEntry, state *raft_log.CompactionJobState) {
	if e.Status == 0 && state.Status != 0 {
		// Job given a status.
		q.stats.assignedTotal++
	}
	if e.Status != 0 && e.Token != state.Token {
		// Token change.
		q.stats.reassignedTotal++
	}
	e.CompactionJobState = state
	heap.Fix(q.jobs, e.index)
}

func (q *jobQueue) delete(e *jobEntry) {
	heap.Remove(q.jobs, e.index)
}

func (q *jobQueue) clone() priorityJobQueue {
	c := make(priorityJobQueue, q.jobs.Len())
	for j, job := range *q.jobs {
		jobCopy := *job
		c[j] = &jobCopy
	}
	return c
}

// The function determines the scheduling order of the jobs.
func compareJobs(a, b *jobEntry) int {
	// Pick jobs in the "initial" (unspecified) state first.
	if a.Status != b.Status {
		return int(a.Status) - int(b.Status)
	}
	// Faulty jobs should wait. Our aim is to put them at the
	// end of the queue, after all the jobs we may consider
	// for assigment.
	if a.Failures != b.Failures {
		return int(a.Failures) - int(b.Failures)
	}
	// Jobs with earlier deadlines should go first.
	// A job that has been just added has no lease
	// and will always go first.
	if a.LeaseExpiresAt != b.LeaseExpiresAt {
		return int(a.LeaseExpiresAt) - int(b.LeaseExpiresAt)
	}
	// Tiebreaker: the job name must not bias the order.
	return strings.Compare(a.Name, b.Name)
}

// TODO(kolesnikovae): container/heap is not very efficient,
//  consider implementing own heap, specific to the case.
//  A treap might be suitable as well.

type priorityJobQueue []*jobEntry

func (pq priorityJobQueue) Len() int { return len(pq) }

func (pq priorityJobQueue) Less(i, j int) bool {
	return compareJobs(pq[i], pq[j]) < 0
}

func (pq priorityJobQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityJobQueue) Push(x interface{}) {
	n := len(*pq)
	job := x.(*jobEntry)
	job.index = n
	*pq = append(*pq, job)
}

func (pq *priorityJobQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	job := old[n-1]
	old[n-1] = nil
	job.index = -1
	*pq = old[0 : n-1]
	return job
}
