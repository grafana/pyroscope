package queue

type blockQueue struct {
	blocks map[compactionKey]*stagedBlocks
	head   *batch
	tail   *batch

	batchSize uint32
}

type compactionKey struct {
	// Order of the fields is not important.
	// Can be generalized.
	tenant string
	shard  uint32
}

type stagedBlocks struct {
	bm    map[string]blockRef
	batch *batch
}

// blockRef points to the block in the batch.
type blockRef struct {
	batch *batch
	index int
}

type batch struct {
	next, prev *batch
	compactionKey
	size   uint32
	blocks []string
}

func (q *blockQueue) newBatch(key compactionKey) *batch {
	return &batch{
		compactionKey: key,
		blocks:        make([]string, 0, q.batchSize),
	}
}

func (q *blockQueue) push(key compactionKey, block string) bool {
	sb, ok := q.blocks[key]
	if !ok {
		sb = &stagedBlocks{
			bm:    make(map[string]blockRef),
			batch: q.newBatch(key),
		}
		q.blocks[key] = sb
	} else if _, found := sb.bm[block]; found {
		return false
	}
	sb.bm[block] = blockRef{
		batch: sb.batch,
		index: len(sb.batch.blocks),
	}
	sb.batch.blocks = append(sb.batch.blocks, block)
	sb.batch.size++
	if sb.batch.size >= q.batchSize {
		q.pushBack(sb.batch)
		sb.batch = q.newBatch(key)
	}
	return true
}

func (q *blockQueue) pushBack(b *batch) {
	if q.head == nil {
		q.head = b
		q.tail = b
		return
	}
	q.tail.next = b
	b.prev = q.tail
	q.tail = b
}

func (q *blockQueue) pop() (b *batch) {
	if q.head == nil {
		return nil
	}
	b, q.head = q.head, q.head.next
	return b
}

const removedBlock = ""

func (q *blockQueue) remove(key compactionKey, block ...string) {
	sb, ok := q.blocks[key]
	if !ok {
		return
	}
	for _, b := range block {
		loc, found := sb.bm[b]
		if !found {
			continue
		}
		// We can't change the order of the blocks in the batch,
		// because that would require updating all the block locations.
		loc.batch.blocks[loc.index] = removedBlock
		loc.batch.size--
		if loc.batch.size == 0 {
			if loc.batch.prev != nil {
				loc.batch.prev.next = loc.batch.next
			}
			if loc.batch.next != nil {
				loc.batch.next.prev = loc.batch.prev
			}
			loc.batch.next = nil
			loc.batch.prev = nil
		}
		delete(sb.bm, b)
	}
}
