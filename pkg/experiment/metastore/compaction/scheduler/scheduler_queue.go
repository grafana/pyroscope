package scheduler

import (
	"container/heap"
	"slices"
	"strings"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type jobQueue struct {
	jobs   map[string]*jobEntry
	levels []priorityJobQueue
}

type jobEntry struct {
	index int // The index of the job in the heap.
	*raft_log.CompactionJobState
}

func newJobQueue() *jobQueue {
	return &jobQueue{jobs: make(map[string]*jobEntry)}
}

func (q *jobQueue) level(x uint32) *priorityJobQueue {
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
	q.jobs[state.Name] = j
	heap.Push(q.level(state.CompactionLevel), j)
}

func (q *jobQueue) delete(name string) *raft_log.CompactionJobState {
	if j, exists := q.jobs[name]; exists {
		delete(q.jobs, name)
		return heap.Remove(q.level(j.CompactionLevel), j.index).(*jobEntry).CompactionJobState
	}
	return nil
}

// The function determines the scheduling order of the jobs.
func compareJobs(a, b *jobEntry) int {
	// Pick jobs in the "initial" (unspecified) state first.
	if a.Status != b.Status {
		return int(a.Status) - int(b.Status)
	}
	// Faulty jobs should wait.
	if a.Failures != b.Failures {
		return int(a.Failures) - int(b.Failures)
	}
	// Jobs with earlier deadlines should go first.
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
