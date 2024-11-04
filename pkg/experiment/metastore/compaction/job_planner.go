package compaction

type jobPlanner struct {
	q *planner
	c *batchIter
	b *batch
	l uint32
	i int
}

type jobPlan struct {
	name   string
	tenant string
	shard  uint32
	level  uint32
	blocks []string
}

func (it *jobPlanner) next() *jobPlan {
	var p jobPlan

	// First, determine the tenant shard and level we want to compact.
	// We pick the oldest block batch at the lowest level first, and
	// iterate batches with the same compaction key until we get enough
	//
	for it.l < uint32(len(it.q.levels)) {
		if it.b == nil {
			l := it.q.levels[it.l]
			if l == nil {
				it.l++
				continue
			}
			it.c = l.blockQueue.iter()
			it.b = it.c.next()
			// This batch determines
			// what tenant shard and level
			// we want to compact.
			it.i = 0
			it.l++
			continue
		}
		// Batch and iterator are initialized already.
		if it.i < len(it.b.blocks) && it.b.blocks[it.i] != removedBlock {
			block := it.b.blocks[it.i]
			it.i++
			p.blocks = append(p.blocks, block)

			continue
		}
		it.b = it.c.next()
		it.i = 0
	}
	return nil
}
