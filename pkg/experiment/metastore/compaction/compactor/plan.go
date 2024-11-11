package compactor

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

// plan should be used to prepare the compaction plan update.
// The implementation must have no side effects or alter the
// Compactor in any way.
type plan struct {
	tx  *bbolt.Tx
	cmd *raft.Log

	// Read-only.
	compactor *Compactor
	batches   *batchIter
	blocks    *blockIter

	level uint32
}

func (p *plan) CreateJob() (*raft_log.CompactionJobPlan, error) {
	planned := p.nextJob()
	if planned == nil {
		return nil, nil
	}
	tombstones, err := p.compactor.tombstones.GetTombstones(p.tx, p.cmd)
	if err != nil {
		return nil, err
	}
	job := raft_log.CompactionJobPlan{
		Name:            planned.name,
		Shard:           planned.shard,
		Tenant:          planned.tenant,
		CompactionLevel: planned.level,
		SourceBlocks:    planned.blocks,
		Tombstones:      tombstones,
	}
	return &job, nil
}

type jobPlan struct {
	compactionKey
	name   string
	blocks []string
}

// Plan compaction of the queued blocks. The algorithm is simple:
//   - Iterate block queues from low levels to higher ones.
//   - Find the oldest batch in the order of arrival and try to compact it.
//   - A batch may not translate into a job (e.g., if some blocks have been
//     removed). Therefore, we navigate to the next batch with the same
//     compaction key in this case.
func (p *plan) nextJob() *jobPlan {
	var job jobPlan
	for p.level < uint32(len(p.compactor.queue.levels)) {
		if p.batches == nil {
			level := p.compactor.queue.levels[p.level]
			if level == nil {
				p.level++
				continue
			}
			p.batches = newBatchIter(level)
		}

		b, ok := p.batches.next()
		if !ok {
			// We've done with the current level: no more batches
			// in the in-order queue. Move to the next level.
			p.batches = nil
			p.level++
			continue
		}

		// We've found the oldest batch, it's time to plan a job.
		// Job levels are zero based: L0 job means that it includes blocks
		// with compaction level 0. This can be altered (1-based levels):
		// job.level++
		job.compactionKey = b.staged.key
		job.blocks = slices.Grow(job.blocks, defaultBlockBatchSize)[:0]
		p.blocks.setBatch(b)

		// Once we finish with the current batch blocks, the iterator moves
		// to the next batch–with-the-same-compaction-key, which is not
		// necessarily the next in-order-batch from the batch iterator.
		for {
			block, ok := p.blocks.next()
			if !ok {
				// No more blocks with this compaction key at the level.
				// The current job plan is to be cancelled, and we move
				// on to the next in-order batch.
				break
			}

			job.blocks = append(job.blocks, block)
			if p.compactor.strategy.complete(&job) {
				nameJob(&job)
				return &job
			}
		}
	}

	return nil
}

// Job name is a variable length string that should be globally unique
// and is used as a tiebreaker in the compaction job queue ordering.
func nameJob(plan *jobPlan) {
	// Should be on stack; 16b per block; expected ~20 blocks.
	buf := make([]byte, 0, 512)
	for _, b := range plan.blocks {
		buf = append(buf, b...)
	}
	var name strings.Builder
	name.WriteString(fmt.Sprintf("%x", xxhash.Sum64(buf)))
	name.WriteByte('-')
	name.WriteByte('T')
	name.WriteString(plan.tenant)
	name.WriteByte('-')
	name.WriteByte('S')
	name.WriteString(strconv.FormatUint(uint64(plan.shard), 10))
	name.WriteByte('-')
	name.WriteByte('L')
	name.WriteString(strconv.FormatUint(uint64(plan.level), 10))
	plan.name = name.String()
}
