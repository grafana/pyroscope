package compactor

import (
	"slices"
)

// TODO(kolesnikovae): Stats.

type compactionKey struct {
	// Order of the fields is not important.
	// Can be generalized.
	tenant string
	shard  uint32
	level  uint32
}

type compactionQueue struct {
	strategy Strategy
	levels   []*blockQueue
}

func newCompactionQueue(strategy Strategy) *compactionQueue {
	return &compactionQueue{strategy: strategy}
}

func (q *compactionQueue) push(k compactionKey, e blockEntry) bool {
	return q.stagedBlocks(k).push(e)
}

func (q *compactionQueue) stagedBlocks(k compactionKey) *stagedBlocks {
	s := k.level + 1 // Levels are 0-based.
	if s > uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
	}
	level := q.levels[k.level]
	if level == nil {
		level = newBlockQueue(q.strategy)
		q.levels[k.level] = level
	}
	return level.stagedBlocks(k)
}

// blockQueue stages blocks as they are being added. Once a batch of blocks
// within the compaction key reaches a certain size, it is pushed to the linked
// list in the arrival order and to the compaction key queue.
//
// This allows to iterate over the blocks in the order of arrival within the
// compaction dimension, while maintaining an ability to remove blocks from the
// queue efficiently.
//
// No pop operation is needed for the block queue: the only way blocks leave
// the queue is through explicit removal. Batch and block iterators provide
// the read access.
type blockQueue struct {
	strategy   Strategy
	staged     map[compactionKey]*stagedBlocks
	head, tail *batch
}

type stagedBlocks struct {
	key  compactionKey
	refs map[string]blockRef
	// Queue of blocks sharing this compaction key.
	head *batch
	tail *batch
	// Incomplete batch of blocks.
	batch *batch
	// Global queue.
	queue *blockQueue
}

// blockRef points to the block in the batch.
type blockRef struct {
	batch *batch
	index int
}

type blockEntry struct {
	id string // Block ID.
	// Index of the command in the raft log.
	raftIndex uint64
}

type batch struct {
	blocks []blockEntry
	size   uint32
	// Links to the global batch queue items:
	// the compaction key of batches may differ.
	next, prev *batch
	// Reference to the parent.
	staged *stagedBlocks
	// Reference to the staged blocks that
	// share the same compaction key.
	nextSameKey, prevSameKey *batch
}

func newBlockQueue(strategy Strategy) *blockQueue {
	return &blockQueue{
		staged:   make(map[compactionKey]*stagedBlocks),
		strategy: strategy,
	}
}

func (q *blockQueue) stagedBlocks(k compactionKey) *stagedBlocks {
	staged, ok := q.staged[k]
	if !ok {
		staged = &stagedBlocks{
			queue: q,
			key:   k,
			refs:  make(map[string]blockRef),
		}
		staged.reset()
		q.staged[k] = staged
	}
	return staged
}

func (s *stagedBlocks) push(block blockEntry) bool {
	if _, found := s.refs[block.id]; found {
		return false
	}
	s.refs[block.id] = blockRef{batch: s.batch, index: len(s.batch.blocks)}
	s.batch.blocks = append(s.batch.blocks, block)
	s.batch.size++
	if s.queue.strategy.flush(s.batch) {
		s.queue.pushBatch(s.batch)
		s.reset()
	}
	return true
}

func (s *stagedBlocks) reset() {
	// TODO(kolesnikovae): get from pool.
	s.batch = &batch{
		blocks: make([]blockEntry, 0, defaultBlockBatchSize),
		staged: s,
	}
}

var zeroBlockEntry blockEntry

func (s *stagedBlocks) delete(block string) blockEntry {
	ref, found := s.refs[block]
	if !found {
		return zeroBlockEntry
	}
	// We can't change the order of the blocks in the batch,
	// because that would require updating all the block locations.
	e := ref.batch.blocks[ref.index]
	ref.batch.blocks[ref.index] = zeroBlockEntry
	ref.batch.size--
	if ref.batch.size == 0 {
		s.queue.removeBatch(ref.batch)
		// TODO(kolesnikovae): return to pool.
	}
	delete(s.refs, block)
	return e
}

func (q *blockQueue) remove(key compactionKey, block ...string) {
	staged, ok := q.staged[key]
	if !ok {
		return
	}
	for _, b := range block {
		staged.delete(b)
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

	// Same for the queue of batches
	// with matching compaction key.

	if b.staged.tail != nil {
		b.staged.tail.nextSameKey = b
		b.prevSameKey = b.staged.tail
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

	// Same for the queue of batches
	// with matching compaction key.

	if b.prevSameKey != nil {
		b.prevSameKey.nextSameKey = b.nextSameKey
	} else {
		// This is the head.
		b.staged.head = b.nextSameKey
	}
	if b.nextSameKey != nil {
		b.nextSameKey.prevSameKey = b.prevSameKey
	} else {
		// This is the tail.
		b.staged.tail = b.nextSameKey
	}
	b.nextSameKey = nil
	b.prevSameKey = nil
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

// batchIter iterates over the batches in the queue, in the order of arrival
// within the compaction key. It's guaranteed that returned blocks are unique
// across all batched.
type blockIter struct {
	visited map[string]struct{}
	batch   *batch
	i       int
}

func newBlockIter() *blockIter {
	// Assuming that block IDs (16b ULID) are globally unique.
	// We could achieve the same with more efficiency by marking visited
	// batches. However, marking visited blocks seems to be more robust,
	// and the size of the map is expected to be small.
	visited := make(map[string]struct{}, 64)
	visited[zeroBlockEntry.id] = struct{}{}
	return &blockIter{visited: visited}
}

func (it *blockIter) setBatch(b *batch) {
	it.batch = b
	it.i = 0
}

func (it *blockIter) next() (string, bool) {
	for it.batch != nil {
		if it.i >= len(it.batch.blocks) {
			it.setBatch(it.batch.nextSameKey)
			continue
		}
		b := it.batch.blocks[it.i]
		if _, visited := it.visited[b.id]; visited {
			it.i++
			continue
		}
		it.visited[b.id] = struct{}{}
		it.i++
		return b.id, true
	}
	return "", false
}
