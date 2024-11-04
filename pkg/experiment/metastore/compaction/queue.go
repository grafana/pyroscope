package compaction

import (
	"slices"
	"strings"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type queue struct {
	strategy compactionStrategy
	levels   []*compactionLevel
}

type compactionLevel struct {
	blockQueue *blockQueue
}

func newQueue(strategy compactionStrategy) *queue {
	return &queue{strategy: strategy}
}

func (q *queue) level(x uint32) *compactionLevel {
	s := x + 1 // Levels are 0-based.
	if s > uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
		q.levels[x] = &compactionLevel{
			blockQueue: newBlockQueue(q.strategy),
		}
	}
	return q.levels[x]
}

func (q *queue) lookupLevel(x uint32) (*compactionLevel, bool) {
	if x >= uint32(len(q.levels)) {
		return nil, false
	}
	return q.levels[x], true
}

func (q *queue) enqueue(md *metastorev1.BlockMeta) bool {
	if !q.strategy.canCompact(md) {
		return false
	}
	k := compactionKey{
		tenant: md.TenantId,
		shard:  md.Shard,
		level:  md.CompactionLevel,
	}
	return q.level(k.level).blockQueue.push(k, md.Id)
}

func (q *queue) remove(k compactionKey, blocks ...string) {
	level, ok := q.lookupLevel(k.level)
	if ok {
		level.blockQueue.remove(k, blocks...)
	}
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
