package compactor

import (
	"container/heap"
	"slices"
	"strings"

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

// TODO: check nil queue, and non-zero index.

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

func (q *jobQueue) delete(name string) {
	if j, exists := q.jobs[name]; exists {
		heap.Remove(q.level(j.CompactionLevel), q.jobs[j.Name].index)
	}
}

// TODO(kolesnikovae): container/heap is not very efficient,
//  consider implementing own heap, specific to the case.

// The function determines the scheduling order of the jobs.
func compareJobs(a, b *jobEntry) int {
	// Pick jobs in the "initial" (unspecified) state first.
	if a.Status != b.Status {
		return int(a.Status - b.Status)
	}
	// Faulty jobs should wait.
	if a.Failures != b.Failures {
		return int(a.Failures - b.Failures)
	}
	// Jobs with earlier deadlines should go first.
	if a.LeaseExpiresAt != b.LeaseExpiresAt {
		return int(a.LeaseExpiresAt - b.LeaseExpiresAt)
	}
	// Tiebreaker: the job name must not bias the order.
	return strings.Compare(a.Name, b.Name)
}

type priorityQueue []*jobEntry

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return compareJobs(pq[i], pq[j]) < 0
}

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
