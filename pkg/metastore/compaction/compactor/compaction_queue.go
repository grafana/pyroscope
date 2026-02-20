package compactor

import (
	"container/heap"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/util"
)

const defaultBlockBatchSize = 20

type compactionKey struct {
	// Order of the fields is not important.
	// Can be generalized.
	tenant string
	shard  uint32
	level  uint32
}

type compactionQueue struct {
	config         Config
	registerer     prometheus.Registerer
	levels         []*blockQueue
	globalStats    *globalQueueStats
	statsCollector *globalQueueStatsCollector
}

// blockQueue stages blocks as they are being added. Once a batch of blocks
// within the compaction key reaches a certain size or age, it is pushed to
// the linked list in the arrival order and to the compaction key queue.
//
// This allows to iterate over the blocks in the order of arrival within the
// compaction dimension, while maintaining an ability to remove blocks from the
// queue efficiently.
//
// No pop operation is needed for the block queue: the only way blocks leave
// the queue is through explicit removal. Batch and block iterators provide
// the read access.
type blockQueue struct {
	config      Config
	registerer  prometheus.Registerer
	staged      map[compactionKey]*stagedBlocks
	globalStats *globalQueueStats
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
	createdAt  int64
}

func newCompactionQueue(config Config, registerer prometheus.Registerer) *compactionQueue {
	globalStats := newGlobalQueueStats(len(config.Levels))
	q := &compactionQueue{
		config:      config,
		registerer:  registerer,
		globalStats: globalStats,
	}
	if registerer != nil {
		q.statsCollector = newGlobalQueueStatsCollector(q)
		util.RegisterOrGet(registerer, q.statsCollector)
	}
	return q
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
	staged.updatedAt = e.AppendedAt
	pushed := staged.push(blockEntry{
		id:    e.ID,
		index: e.Index,
	})
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
		level = newBlockQueue(q.config, q.registerer, q.globalStats)
		q.levels[l] = level
	}
	return level
}

func newBlockQueue(config Config, registerer prometheus.Registerer, globalStats *globalQueueStats) *blockQueue {
	return &blockQueue{
		config:      config,
		registerer:  registerer,
		staged:      make(map[compactionKey]*stagedBlocks),
		globalStats: globalStats,
		updates:     new(priorityBlockQueue),
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
		q.globalStats.AddQueues(k, 1)
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
	if s.heapIndex < 0 || s.heapIndex >= q.updates.Len() {
		panic("bug: attempt to delete compaction queue with an invalid priority index")
	}
	heap.Remove(q.updates, s.heapIndex)
	q.globalStats.AddQueues(s.key, -1)
}

func (s *stagedBlocks) push(block blockEntry) bool {
	if _, found := s.refs[block.id]; found {
		s.stats.rejected.Add(1)
		return false
	}
	s.refs[block.id] = blockRef{batch: s.batch, index: len(s.batch.blocks)}
	s.batch.blocks = append(s.batch.blocks, block)
	if s.batch.size == 0 {
		s.batch.createdAt = s.updatedAt
	}
	s.batch.size++
	s.stats.blocks.Add(1)
	s.queue.globalStats.AddBlocks(s.key, 1)
	if s.queue.config.exceedsMaxSize(s.batch) ||
		s.queue.config.exceedsMaxAge(s.batch, s.updatedAt) {
		s.flush()
	}
	return true
}

func (s *stagedBlocks) flush() {
	var flushed bool
	s.batch.flush.Do(func() {
		if !s.queue.pushBatch(s.batch) {
			panic("bug: attempt to detach the compaction queue head")
		}
		flushed = true
	})
	if !flushed {
		panic("bug: attempt to flush a compaction queue batch twice")
	}
	s.resetBatch()
}

func (s *stagedBlocks) resetBatch() {
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
	s.queue.globalStats.AddBlocks(s.key, -1)
	if ref.batch.size == 0 {
		if ref.batch != s.batch {
			// We should never ever try to delete the staging batch from the
			// queue: it has not been flushed and added to the queue yet.
			//
			// NOTE(kolesnikovae):
			//  It caused a problem because removeBatch mistakenly interpreted
			//  the batch as a head (s.batch.prev == nil), and detached it,
			//  replacing a valid head with s.batch.next, which is always nil
			//  at this point; it made the queue look empty for the reader,
			//  because the queue is read from the head.
			//
			//  The only way we may end up here if blocks are removed from the
			//  staging batch. Typically, blocks are not supposed to be removed
			//  from there before they left the queue (i.e., flushed to the
			//  global queue).
			//
			//  In practice, the compactor is distributed and has multiple
			//  replicas: the leader instance could have already decided to
			//  flush the blocks, and now they should be removed from all
			//  instances. Due to a bug in time-based flushing (when it stops
			//  working), it was possible that after the leader restarts and
			//  recovers time-based flushing locally, it would desire to flush
			//  the oldest batch. Consequently, the follower instances, where
			//  the batch is still in staging, would need to do the same.
			if !s.queue.removeBatch(ref.batch) {
				panic("bug: attempt to remove a batch that is not in the compaction queue")
			}
		}
	}
	delete(s.refs, block)
	if len(s.refs) == 0 {
		// This is the last block with the given compaction key, so we want to
		// remove the staging structure. It's fine to delete it from the queue
		// at any point: we guarantee that it does not reference any blocks in
		// the queue, and we do not need to flush it anymore.
		s.queue.removeStaged(s)
	}
	return e
}

func (q *blockQueue) pushBatch(b *batch) bool {
	if q.tail != nil {
		q.tail.nextG = b
		b.prevG = q.tail
	} else if q.head == nil {
		q.head = b
	} else {
		return false
	}
	q.tail = b

	// Same for the queue of batches
	// with matching compaction key.

	if b.staged.tail != nil {
		b.staged.tail.next = b
		b.prev = b.staged.tail
	} else if b.staged.head == nil {
		b.staged.head = b
	} else {
		return false
	}
	b.staged.tail = b

	b.staged.stats.batches.Add(1)
	q.globalStats.AddBatches(b.staged.key, 1)
	return true
}

func (q *blockQueue) removeBatch(b *batch) bool {
	if b.prevG != nil {
		b.prevG.nextG = b.nextG
	} else if b == q.head {
		// This is the head.
		q.head = q.head.nextG
	} else {
		return false
	}
	if b.nextG != nil {
		b.nextG.prevG = b.prevG
	} else if b == q.tail {
		// This is the tail.
		q.tail = q.tail.prevG
	} else {
		return false
	}
	b.nextG = nil
	b.prevG = nil

	// Same for the queue of batches
	// with matching compaction key.

	if b.prev != nil {
		b.prev.next = b.next
	} else if b == b.staged.head {
		// This is the head.
		b.staged.head = b.staged.head.next
	} else {
		return false
	}
	if b.next != nil {
		b.next.prev = b.prev
	} else if b == b.staged.tail {
		// This is the tail.
		b.staged.tail = b.staged.tail.prev
	} else {
		return false
	}
	b.next = nil
	b.prev = nil

	b.staged.stats.batches.Add(-1)
	q.globalStats.AddBatches(b.staged.key, -1)
	return true
}

func (q *blockQueue) flushOldest(now int64) {
	if q.updates.Len() == 0 {
		panic("bug: compaction queue has empty priority queue")
	}
	// Peek the oldest staging batch in the priority queue (min-heap).
	oldest := (*q.updates)[0]
	if !q.config.exceedsMaxAge(oldest.batch, now) {
		return
	}
	// It's possible that the staging batch is empty: it's only removed
	// from the queue when the last block with the given compaction key is
	// removed, including ones flushed to the global queue. Therefore, we
	// should not pop it from the queue, but update its index in the heap.
	// Otherwise, if the staging batch has not been removed from the queue
	// yet i.e., references some blocks in the compaction queue (it's rare
	// but not impossible), time-based flush will stop working for it.
	if oldest.batch.size > 0 {
		oldest.flush()
	}
	oldest.updatedAt = now
	heap.Fix(q.updates, oldest.heapIndex)
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

func (i *batchIter) reset(b *batch) { i.batch = b }

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

func (it *blockIter) more() bool {
	if it.batch == nil {
		return false
	}
	return it.i < len(it.batch.blocks)
}

func (it *blockIter) peek() (string, bool) {
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
		return entry.id, true
	}
	return "", false
}

func (it *blockIter) advance() {
	entry := it.batch.blocks[it.i]
	it.visited[entry.id] = struct{}{}
	it.i++
}
