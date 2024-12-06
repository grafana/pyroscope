package compactor

import (
	"container/heap"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/util"
)

type compactionKey struct {
	// Order of the fields is not important.
	// Can be generalized.
	tenant string
	shard  uint32
	level  uint32
}

type compactionQueue struct {
	strategy   Strategy
	registerer prometheus.Registerer
	levels     []*blockQueue
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
	registerer prometheus.Registerer
	staged     map[compactionKey]*stagedBlocks
	// Batches ordered by arrival.
	head, tail *batch
	// Priority queue by last update: we need to flush
	// incomplete batches once they stop updating.
	updates *priorityBlockQueue
}

// stagedBlocks is a queue of blocks sharing the same compaction key.
type stagedBlocks struct {
	key compactionKey
	// Local queue (blocks sharing this compaction key).
	head, tail *batch
	// Parent block queue (global).
	queue *blockQueue
	// Incomplete batch of blocks.
	batch *batch
	// Map of block IDs to their locations in batches.
	refs      map[string]blockRef
	stats     *queueStats
	collector *queueStatsCollector
	// Parent block queue maintains a priority queue of
	// incomplete batches by the last update time.
	heapIndex int
	updatedAt int64
}

type queueStats struct {
	blocks   atomic.Int32
	batches  atomic.Int32
	rejected atomic.Int32
	missed   atomic.Int32
}

// blockRef points to the block in the batch.
type blockRef struct {
	batch *batch
	index int
}

type blockEntry struct {
	id    string // Block ID.
	index uint64 // Index of the command in the raft log.
}

type batch struct {
	flush  sync.Once
	size   uint32
	blocks []blockEntry
	// Reference to the parent.
	staged *stagedBlocks
	// Links to the global batch queue items:
	// the compaction key of batches may differ.
	nextG, prevG *batch
	// Links to the local batch queue items:
	// batches that share the same compaction key.
	next, prev *batch
}

func newCompactionQueue(strategy Strategy, registerer prometheus.Registerer) *compactionQueue {
	return &compactionQueue{
		strategy:   strategy,
		registerer: registerer,
	}
}

func (q *compactionQueue) reset() {
	for _, level := range q.levels {
		if level != nil {
			for _, s := range level.staged {
				level.removeStaged(s)
			}
		}
	}
	clear(q.levels)
	q.levels = q.levels[:0]
}

func (q *compactionQueue) push(e compaction.BlockEntry) bool {
	level := q.blockQueue(e.Level)
	staged := level.stagedBlocks(compactionKey{
		tenant: e.Tenant,
		shard:  e.Shard,
		level:  e.Level,
	})
	pushed := staged.push(blockEntry{
		id:    e.ID,
		index: e.Index,
	})
	staged.updatedAt = e.AppendedAt
	heap.Fix(level.updates, staged.heapIndex)
	level.flushOldest(e.AppendedAt)
	return pushed
}

func (q *compactionQueue) blockQueue(l uint32) *blockQueue {
	s := l + 1 // Levels are 0-based.
	if s > uint32(len(q.levels)) {
		q.levels = slices.Grow(q.levels, int(s))[:s]
	}
	level := q.levels[l]
	if level == nil {
		level = newBlockQueue(q.strategy, q.registerer)
		q.levels[l] = level
	}
	return level
}

func newBlockQueue(strategy Strategy, registerer prometheus.Registerer) *blockQueue {
	return &blockQueue{
		strategy:   strategy,
		registerer: registerer,
		staged:     make(map[compactionKey]*stagedBlocks),
		updates:    new(priorityBlockQueue),
	}
}

func (q *blockQueue) stagedBlocks(k compactionKey) *stagedBlocks {
	staged, ok := q.staged[k]
	if !ok {
		staged = &stagedBlocks{
			queue: q,
			key:   k,
			refs:  make(map[string]blockRef),
			stats: new(queueStats),
		}
		staged.resetBatch()
		q.staged[k] = staged
		heap.Push(q.updates, staged)
		if q.registerer != nil {
			staged.collector = newQueueStatsCollector(staged)
			util.RegisterOrGet(q.registerer, staged.collector)
		}
	}
	return staged
}

func (q *blockQueue) removeStaged(s *stagedBlocks) {
	if s.collector != nil {
		q.registerer.Unregister(s.collector)
	}
	delete(q.staged, s.key)
	if s.heapIndex < 0 {
		// We usually end up here since s has already been evicted
		// from the priority queue via Pop due to its age.
		return
	}
	if s.heapIndex >= q.updates.Len() {
		// Should not be possible.
		return
	}
	heap.Remove(q.updates, s.heapIndex)
}

func (s *stagedBlocks) push(block blockEntry) bool {
	if _, found := s.refs[block.id]; found {
		s.stats.rejected.Add(1)
		return false
	}
	s.refs[block.id] = blockRef{batch: s.batch, index: len(s.batch.blocks)}
	s.batch.blocks = append(s.batch.blocks, block)
	s.batch.size++
	s.stats.blocks.Add(1)
	if s.queue.strategy.flush(s.batch) && !s.flush() {
		// An attempt to flush the same batch twice.
		// Should not be possible.
		return false
	}
	return true
}

func (s *stagedBlocks) flush() (flushed bool) {
	s.batch.flush.Do(func() {
		s.queue.pushBatch(s.batch)
		flushed = true
	})
	s.resetBatch()
	return flushed
}

func (s *stagedBlocks) resetBatch() {
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
		s.stats.missed.Add(1)
		return zeroBlockEntry
	}
	// We can't change the order of the blocks in the batch,
	// because that would require updating all the block locations.
	e := ref.batch.blocks[ref.index]
	ref.batch.blocks[ref.index] = zeroBlockEntry
	ref.batch.size--
	s.stats.blocks.Add(-1)
	if ref.batch.size == 0 {
		s.queue.removeBatch(ref.batch)
		// TODO(kolesnikovae): return to pool.
	}
	delete(s.refs, block)
	if len(s.refs) == 0 {
		s.queue.removeStaged(s)
	}
	return e
}

func (q *blockQueue) pushBatch(b *batch) {
	if q.tail != nil {
		q.tail.nextG = b
		b.prevG = q.tail
	} else {
		q.head = b
	}
	q.tail = b

	// Same for the queue of batches
	// with matching compaction key.

	if b.staged.tail != nil {
		b.staged.tail.next = b
		b.prev = b.staged.tail
	} else {
		b.staged.head = b
	}
	b.staged.tail = b
	b.staged.stats.batches.Add(1)
}

func (q *blockQueue) removeBatch(b *batch) {
	if b.prevG != nil {
		b.prevG.nextG = b.nextG
	} else {
		// This is the head.
		q.head = b.nextG
	}
	if b.nextG != nil {
		b.nextG.prevG = b.prevG
	} else {
		// This is the tail.
		q.tail = b.prevG
	}
	b.nextG = nil
	b.prevG = nil

	// Same for the queue of batches
	// with matching compaction key.

	if b.prev != nil {
		b.prev.next = b.next
	} else {
		// This is the head.
		b.staged.head = b.next
	}
	if b.next != nil {
		b.next.prev = b.prev
	} else {
		// This is the tail.
		b.staged.tail = b.next
	}
	b.next = nil
	b.prev = nil
	b.staged.stats.batches.Add(-1)
}

func (q *blockQueue) flushOldest(now int64) {
	if q.updates.Len() == 0 {
		// Should not be possible.
		return
	}
	oldest := (*q.updates)[0]
	if !q.strategy.flushByAge(oldest.batch, now) {
		return
	}
	heap.Pop(q.updates)
	oldest.flush()
}

type priorityBlockQueue []*stagedBlocks

func (pq priorityBlockQueue) Len() int { return len(pq) }

func (pq priorityBlockQueue) Less(i, j int) bool {
	return pq[i].updatedAt < pq[j].updatedAt
}

func (pq priorityBlockQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].heapIndex = i
	pq[j].heapIndex = j
}

func (pq *priorityBlockQueue) Push(x interface{}) {
	n := len(*pq)
	staged := x.(*stagedBlocks)
	staged.heapIndex = n
	*pq = append(*pq, staged)
}

func (pq *priorityBlockQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	staged := old[n-1]
	old[n-1] = nil
	staged.heapIndex = -1
	*pq = old[0 : n-1]
	return staged
}

func newBatchIter(q *blockQueue) *batchIter { return &batchIter{batch: q.head} }

// batchIter iterates over the batches in the queue, in the order of arrival.
type batchIter struct{ batch *batch }

func (i *batchIter) next() (*batch, bool) {
	if i.batch == nil {
		return nil, false
	}
	b := i.batch
	i.batch = i.batch.nextG
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
			it.setBatch(it.batch.next)
			continue
		}
		entry := it.batch.blocks[it.i]
		if _, visited := it.visited[entry.id]; visited {
			it.i++
			continue
		}
		it.visited[entry.id] = struct{}{}
		it.i++
		return entry.id, true
	}
	return "", false
}
