package compaction

// blockQueue stages blocks as they are being added.
// Once a batch of blocks within the compaction key
// dimension reaches a certain size, it is pushed to
// the linked list in the order of arrival.
//
// An iterator provides a way to list the queued blocks.
//
// No pop operation is needed for the block queue:
// the only way they leave the queue is through explicit
// removal.
type blockQueue struct {
	head, tail *batch
	staged     map[compactionKey]*stagedBlocks
	strategy   batchStrategy
	// TODO(kolesnikovae): batch pool.
}

func newBlockQueue(strategy batchStrategy) *blockQueue {
	if strategy == nil {
		strategy = blockBatchSize{num: defaultBlockBatchSize}
	}
	return &blockQueue{
		staged:   make(map[compactionKey]*stagedBlocks),
		strategy: strategy,
	}
}

type compactionKey struct {
	// Order of the fields is not important.
	// Can be generalized.
	tenant string
	shard  uint32
}

type stagedBlocks struct {
	refs  map[string]blockRef
	batch *batch
}

// blockRef points to the block in the batch.
type blockRef struct {
	batch *batch
	index int
}

type batch struct {
	compactionKey
	size   uint32
	blocks []string
	// Links to the batch queue items:
	// the compaction key may differ.
	next, prev *batch
}

func (q *blockQueue) push(k compactionKey, block string) bool {
	staged := q.stagedBlocks(k)
	// Flush staged if we can't add more blocks.
	// For example, if the batch is too old.
	if q.strategy.flush(staged.batch) {
		q.pushBatch(staged.batch)
		staged.batch = q.newBatch(k)
	}
	if !staged.pushBlock(block) {
		return false
	}
	if q.strategy.flush(staged.batch) {
		q.pushBatch(staged.batch)
		staged.batch = q.newBatch(k)
	}
	return true
}

func (q *blockQueue) stagedBlocks(k compactionKey) *stagedBlocks {
	staged, ok := q.staged[k]
	if !ok {
		staged = &stagedBlocks{
			refs:  make(map[string]blockRef),
			batch: q.newBatch(k),
		}
		q.staged[k] = staged
	}
	return staged
}

func (q *blockQueue) newBatch(key compactionKey) *batch {
	return &batch{
		compactionKey: key,
		blocks:        make([]string, 0, defaultBlockBatchSize),
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

const removedBlock = ""

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
		ref.batch.blocks[ref.index] = removedBlock
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
}

func (q *blockQueue) iter() *batchIter { return &batchIter{batch: q.head} }

type batchIter struct{ batch *batch }

func (i *batchIter) next() *batch {
	if i.batch == nil {
		return nil
	}
	b := i.batch
	i.batch = i.batch.next
	return b
}
