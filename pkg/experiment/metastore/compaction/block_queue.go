package compaction

type compactionKey struct {
	// Order of the fields is not important.
	// Can be generalized.
	tenant string
	shard  uint32
	level  uint32
}

// blockQueue stages blocks as they are being added.
// Once a batch of blocks within the compaction key
// dimension reaches a certain size, it is pushed to
// the linked list in the order of arrival.
//
// No pop operation is needed for the block queue:
// the only way they leave the queue is through explicit
// removal.
//
// Batch and block iterators provide the read access.
type blockQueue struct {
	head, tail *batch
	staged     map[compactionKey]*stagedBlocks
	strategy   compactionStrategy
}

func newBlockQueue(strategy compactionStrategy) *blockQueue {
	if strategy == nil {
		strategy = defaultCompactionStrategy
	}
	return &blockQueue{
		staged:   make(map[compactionKey]*stagedBlocks),
		strategy: strategy,
	}
}

type stagedBlocks struct {
	compactionKey
	refs map[string]blockRef
	// Incomplete batch of blocks.
	batch *batch
	// Blocks produced for the compaction key.
	head *batch
	tail *batch
}

// blockRef points to the block in the batch.
type blockRef struct {
	batch *batch
	index int
}

type batch struct {
	blocks []string
	size   uint32
	// Links to the global batch queue items:
	// the compaction key may differ.
	next, prev *batch
	// Reference to the parent.
	staged *stagedBlocks
	// Reference to the staged blocks that
	// share the same compaction key.
	nextStaged, prevStaged *batch
}

func (q *blockQueue) push(k compactionKey, block string) bool {
	staged := q.stagedBlocks(k)
	if !staged.pushBlock(block) {
		return false
	}
	if q.strategy.flush(staged.batch) {
		q.pushBatch(staged.batch)
		q.reset(staged)
	}
	return true
}

func (q *blockQueue) stagedBlocks(k compactionKey) *stagedBlocks {
	staged, ok := q.staged[k]
	if !ok {
		staged = &stagedBlocks{
			compactionKey: k,
			refs:          make(map[string]blockRef),
		}
		q.reset(staged)
		q.staged[k] = staged
	}
	return staged
}

func (q *blockQueue) reset(s *stagedBlocks) {
	s.batch = &batch{
		// TODO(kolesnikovae): pool.
		blocks: make([]string, 0, defaultBlockBatchSize),
		staged: s,
	}
}

func (s *stagedBlocks) pushBlock(block string) bool {
	if _, found := s.refs[block]; found {
		return false
	}
	s.refs[block] = blockRef{batch: s.batch, index: len(s.batch.blocks)}
	s.batch.blocks = append(s.batch.blocks, block)
	s.batch.size++
	return true
}

const removedBlockSentinel = ""

func (q *blockQueue) remove(key compactionKey, block ...string) {
	staged, ok := q.staged[key]
	if !ok {
		return
	}
	for _, b := range block {
		ref, found := staged.refs[b]
		if !found {
			continue
		}
		// We can't change the order of the blocks in the batch,
		// because that would require updating all the block locations.
		ref.batch.blocks[ref.index] = removedBlockSentinel
		ref.batch.size--
		if ref.batch.size == 0 {
			q.removeBatch(ref.batch)
		}
		delete(staged.refs, b)
	}
}

func (q *blockQueue) pushBatch(b *batch) {
	if q.tail != nil {
		q.tail.next = b
		b.prev = q.tail
	} else {
		q.head = b
	}
	q.tail = b

	// Same for the staged list.

	if b.staged.tail != nil {
		b.staged.tail.nextStaged = b
		b.prevStaged = b.staged.tail
	} else {
		b.staged.head = b
	}
	b.staged.tail = b
}

func (q *blockQueue) removeBatch(b *batch) {
	if b.prev != nil {
		b.prev.next = b.next
	} else {
		// This is the head.
		q.head = b.next
	}

	if b.next != nil {
		b.next.prev = b.prev
	} else {
		// This is the tail.
		q.tail = b.prev
	}

	b.next = nil
	b.prev = nil

	// Same for the staged list.

	if b.prevStaged != nil {
		b.prevStaged.nextStaged = b.nextStaged
	} else {
		// This is the head.
		b.staged.head = b.nextStaged
	}

	if b.nextStaged != nil {
		b.nextStaged.prevStaged = b.prevStaged
	} else {
		// This is the tail.
		b.staged.tail = b.nextStaged
	}
	b.nextStaged = nil
	b.prevStaged = nil
}

func newBatchIter(q *blockQueue) *batchIter { return &batchIter{batch: q.head} }

// batchIter iterates over the batches in the queue, in the order of arrival.
type batchIter struct{ batch *batch }

func (i *batchIter) next() (*batch, bool) {
	if i.batch == nil {
		return nil, false
	}
	b := i.batch
	i.batch = i.batch.next
	return b, b != nil
}

// batchIter iterates over the batches in the queue,
// in the order of arrival within the compaction key.
type blockIter struct {
	batch *batch
	i     int
}

func newBlockIter(b *batch) *blockIter { return &blockIter{batch: b} }

func (it *blockIter) next() (string, bool) {
	for it.batch != nil {
		if it.i >= len(it.batch.blocks) {
			it.batch = it.batch.nextStaged
			it.i = 0
			continue
		}
		b := it.batch.blocks[it.i]
		if b == removedBlockSentinel {
			it.i++
			continue
		}
		it.i++
		return b, true
	}
	return removedBlockSentinel, false
}

func (it *blockIter) seek(n int) bool {
	for i := 0; i < n; i++ {
		it.next()
	}
	return it.batch != nil
}
