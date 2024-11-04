package compaction

import (
	"slices"
	"strings"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type planner struct {
	strategy compactionStrategy
	levels   []*compactionLevel
}

type compactionLevel struct {
	blockQueue *blockQueue
}

func newCompactionPlanner(strategy compactionStrategy) *planner {
	return &planner{strategy: strategy}
}

func (p *planner) level(x uint32) *compactionLevel {
	s := x + 1 // Levels are 0-based.
	if s >= uint32(len(p.levels)) {
		p.levels = slices.Grow(p.levels, int(s))[:s]
		p.levels[x] = &compactionLevel{
			blockQueue: newBlockQueue(p.strategy.batchStrategy(x)),
		}
	}
	return p.levels[x]
}

func (p *planner) enqueueBlock(md *metastorev1.BlockMeta) bool {
	if !p.strategy.canCompact(md) {
		return false
	}
	k := compactionKey{tenant: md.TenantId, shard: md.Shard}
	return p.level(md.CompactionLevel).blockQueue.push(k, md.Id)
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
