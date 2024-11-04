package compaction

import (
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
)

type jobPlan struct {
	name   string
	tenant string
	shard  uint32
	level  uint32
	blocks []string
}

type jobPlanner struct {
	strategy compactionStrategy

	queue   *queue
	level   uint32
	batches *batchIter
	visited map[compactionKey]struct{}

	batch  *batch
	blocks *blockIter
}

// Plan compaction of the queued blocks. The algorithm is simple:
//   - Iterate block queues from low levels to higher ones.
//   - Find the oldest batch in the order of arrival and try to compact it.
//   - A batch may not translate into a job (e.g., if some blocks have been
//     removed). Therefore, we navigate to the next batch with the same
//     compaction key in this case.
func (p *jobPlanner) nextJob() *jobPlan {
	var job jobPlan
	for p.level < uint32(len(p.queue.levels)) {
		if p.batches == nil {
			c := p.queue.levels[p.level]
			if c == nil {
				p.level++
				continue
			}
			p.batches = newBatchIter(c.blockQueue)
			clear(p.visited)
		}

		var ok bool
		p.batch, ok = p.batches.next()
		if !ok {
			// We've done with the current level: no more batches
			// in the in-order queue. Move to the next level.
			p.batches = nil
			p.level++
			continue
		}
		if !p.visit(p.batch.staged.compactionKey) {
			continue
		}

		// We've found the oldest batch, it's time to plan a job.
		if p.blocks == nil {
			p.blocks = newBlockIter(p.batch)
		}

		job.level = p.batch.staged.level
		job.tenant = p.batch.staged.tenant
		job.shard = p.batch.staged.shard
		job.blocks = job.blocks[:0]

		// Once we finish with the current batch blocks, the iterator moves
		// to the next batch–with-the-same-compaction-key, which is not
		// necessarily the next in-order-batch from the batch iterator.
		for j := 0; ; j++ {
			block, ok := p.blocks.next()
			if !ok {
				// No more blocks with this compaction key.
				// The current job plan is to be cancelled.
				p.blocks = nil
				break
			}

			if !p.strategy.canAdd(&job, block) && len(job.blocks) > 0 {
				p.blocks = newBlockIter(p.batch)
				p.blocks.seek(j - 1) // Undo next().
				p.nameJobPlan(&job)
				return &job
			}

			job.blocks = append(job.blocks, block)
			if p.strategy.done(&job) {
				p.nameJobPlan(&job)
				return &job
			}
		}
	}

	return nil
}

func (p *jobPlanner) visit(k compactionKey) bool {
	if _, ok := p.visited[k]; ok {
		return false
	}
	p.visited[k] = struct{}{}
	return true
}

func (p *jobPlanner) nameJobPlan(plan *jobPlan) {
	// Should be on stack; 16b per block; expected ~20 blocks.
	buf := make([]byte, 0, 512)
	for _, b := range plan.blocks {
		buf = append(buf, b...)
	}
	var name strings.Builder
	name.WriteString(strconv.FormatUint(xxhash.Sum64(buf), 10))
	name.WriteByte('-')
	name.WriteByte('S')
	name.WriteString(strconv.FormatUint(uint64(plan.shard), 10))
	name.WriteByte('-')
	name.WriteByte('L')
	name.WriteString(strconv.FormatUint(uint64(plan.level), 10))
	plan.name = name.String()
}
