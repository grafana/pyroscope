package queue

import (
	"slices"
	"strings"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type compactionQueue struct {
	jobs   map[string]*jobQueueEntry
	levels []*compactionLevel
}

type jobQueueEntry struct {
	// The index of the job in the queue.
	index int
	*raft_log.CompactionJobState
}

type compactionLevel struct {
	jobQueue   []*raft_log.CompactionJobState
	blockQueue *blockQueue
}

func (q *compactionQueue) level(x uint32) *compactionLevel {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
		q.levels[x] = new(compactionLevel)
	}
	return q.levels[x]
}

func compareJobs(a *raft_log.CompactionJobState, b *raft_log.CompactionJobState) int {
	if a.Status != b.Status {
		// Pick jobs in the "initial" (unspecified) state first.
		return int(a.Status) - int(b.Status)
	}
	if a.LeaseExpiresAt != b.LeaseExpiresAt {
		// Jobs with earlier deadlines should be at the top.
		return int(a.LeaseExpiresAt) - int(b.LeaseExpiresAt)
	}
	return strings.Compare(a.Name, b.Name)
}

func (q *compactionQueue) enqueueJob(job *raft_log.CompactionJobState) bool {
	_, ok := q.jobs[job.Name]
	if ok {
		return false
	}
	level := q.level(job.CompactionLevel)
	n, ok := slices.BinarySearchFunc(level.jobQueue, job, compareJobs)
	if ok {
		return false
	}
	level.jobQueue = slices.Insert(level.jobQueue, n, job)
	q.jobs[job.Name] = &jobQueueEntry{CompactionJobState: job, index: n}
	return true
}

func (q *compactionQueue) peekJob(level uint32, i int) *raft_log.CompactionJobState {
	if i >= len(q.levels) {
		return nil
	}
	if i >= len(q.levels[level].jobQueue) {
		return nil
	}
	return q.levels[level].jobQueue[i]
}

func (q *compactionQueue) enqueueBlock(md *metastorev1.BlockMeta) bool {
	k := compactionKey{tenant: md.TenantId, shard: md.Shard}
	return q.level(md.CompactionLevel).blockQueue.push(k, md.Id)
}

type jobIter struct {
	q *compactionQueue
	l uint32
	i int
}

func (x *jobIter) next() *raft_log.CompactionJobState {
	for x.l < uint32(len(x.q.levels)) {
		job := x.q.peekJob(x.l, x.i)
		x.i++
		if job != nil {
			return job
		}
		x.l++
		x.i = 0
	}
	return nil
}
