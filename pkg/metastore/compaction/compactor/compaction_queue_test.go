package compactor

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/metastore/compaction"
)

func testBlockEntry(id int) blockEntry { return blockEntry{id: strconv.Itoa(id)} }

func testBlockQueue(cfg Config) *blockQueue {
	stats := newGlobalQueueStats(len(cfg.Levels))
	return newBlockQueue(cfg, nil, stats)
}

func TestBlockQueue_Push(t *testing.T) {
	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 3}}})
	key := compactionKey{tenant: "t", shard: 1}

	result := q.stagedBlocks(key).push(testBlockEntry(1))
	require.True(t, result)
	require.Equal(t, 1, len(q.staged[key].batch.blocks))
	assert.Equal(t, testBlockEntry(1), q.staged[key].batch.blocks[0])

	q.stagedBlocks(key).push(testBlockEntry(2))
	q.stagedBlocks(key).push(testBlockEntry(3)) // Staged blocks formed the first batch.
	assert.Equal(t, 0, len(q.staged[key].batch.blocks))
	assert.Equal(t, []blockEntry{testBlockEntry(1), testBlockEntry(2), testBlockEntry(3)}, q.head.blocks)

	q.stagedBlocks(key).push(testBlockEntry(4))
	q.stagedBlocks(key).push(testBlockEntry(5))
	assert.Equal(t, 2, len(q.staged[key].batch.blocks))

	remove(q, key, "1", "2") // Remove the first batch.
	assert.Equal(t, []blockEntry{zeroBlockEntry, zeroBlockEntry, testBlockEntry(3)}, q.head.blocks)
	remove(q, key, "3")
	assert.Nil(t, q.head)

	q.stagedBlocks(key).push(testBlockEntry(6)) // Complete the second batch.
	assert.Equal(t, 0, len(q.staged[key].batch.blocks))

	q.stagedBlocks(key).push(testBlockEntry(7))
	assert.Equal(t, []blockEntry{testBlockEntry(4), testBlockEntry(5), testBlockEntry(6)}, q.head.blocks)
	assert.Equal(t, 1, len(q.staged[key].batch.blocks))
}

func TestBlockQueue_DuplicateBlock(t *testing.T) {
	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 3}}})
	key := compactionKey{tenant: "t", shard: 1}

	require.True(t, q.stagedBlocks(key).push(testBlockEntry(1)))
	require.False(t, q.stagedBlocks(key).push(testBlockEntry(1)))

	assert.Equal(t, 1, len(q.staged[key].batch.blocks))
}

func TestBlockQueue_Remove(t *testing.T) {
	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 3}}})
	key := compactionKey{tenant: "t", shard: 1}
	q.stagedBlocks(key).push(testBlockEntry(1))
	q.stagedBlocks(key).push(testBlockEntry(2))

	remove(q, key, "1")
	require.Empty(t, q.staged[key].batch.blocks[0])

	_, exists := q.staged[key].refs["1"]
	assert.False(t, exists)

	remove(q, key, "2")
	require.Nil(t, q.head)
	require.Nil(t, q.tail)
}

func TestBlockQueue_RemoveNotFound(t *testing.T) {
	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 3}}})
	key := compactionKey{tenant: "t", shard: 1}
	remove(q, key, "1")
	q.stagedBlocks(key).push(testBlockEntry(1))
	remove(q, key, "2")
	q.stagedBlocks(key).push(testBlockEntry(2))
	q.stagedBlocks(key).push(testBlockEntry(3))

	assert.Equal(t, []blockEntry{testBlockEntry(1), testBlockEntry(2), testBlockEntry(3)}, q.head.blocks)
}

func TestBlockQueue_Linking(t *testing.T) {
	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 2}}})
	key := compactionKey{tenant: "t", shard: 1}

	q.stagedBlocks(key).push(testBlockEntry(1))
	q.stagedBlocks(key).push(testBlockEntry(2))
	head, tail := q.head, q.tail
	require.NotNil(t, head)
	assert.Equal(t, head, tail)

	q.stagedBlocks(key).push(testBlockEntry(3))
	assert.NotNil(t, q.tail)
	assert.Equal(t, head, q.head)
	assert.Equal(t, tail, q.tail)
	assert.Equal(t, []blockEntry{testBlockEntry(1), testBlockEntry(2)}, q.head.blocks)
	assert.Equal(t, q.tail.blocks, q.head.blocks)

	q.stagedBlocks(key).push(testBlockEntry(4))
	assert.NotNil(t, q.tail.prevG)
	assert.NotNil(t, q.head.nextG)

	q.stagedBlocks(key).push(testBlockEntry(5))
	q.stagedBlocks(key).push(testBlockEntry(6))
	assert.NotNil(t, q.tail.prevG.prevG)
	assert.NotNil(t, q.head.nextG.nextG)

	t.Run("iterator does not affect the queue", func(t *testing.T) {
		expected := []string{"1", "2", "3", "4", "5", "6"}
		for i := 0; i < 3; i++ {
			collected := make([]string, 0, len(expected))
			iter := newBlockIter()
			iter.setBatch(q.head)
			for {
				b, ok := iter.peek()
				if !ok {
					assert.Equal(t, expected, collected)
					break
				}
				collected = append(collected, b)
				iter.advance()
			}
		}
	})

	t.Run("remove staging batch from the queue", func(t *testing.T) {
		q.stagedBlocks(key).push(testBlockEntry(7))
		// Block 7 is still staged: test that it can be
		// removed without affecting the queue.
		head, tail = q.head, q.tail
		remove(q, key, "7")
		assert.Equal(t, head, q.head)
		assert.Equal(t, tail, q.tail)
	})

	t.Run("empty queue", func(t *testing.T) {
		remove(q, key, "3", "2")
		remove(q, key, "4", "1")
		remove(q, key, "6")
		remove(q, key, "5")
		assert.Nil(t, q.head)
		assert.Nil(t, q.tail)
	})
}

func TestBlockQueue_EmptyQueue(t *testing.T) {
	const (
		numKeys         = 50
		numBlocksPerKey = 100
	)

	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 3}}})
	keys := make([]compactionKey, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = compactionKey{
			tenant: fmt.Sprint(i),
			shard:  uint32(i),
		}
	}

	blocks := make(map[compactionKey][]string)
	for _, key := range keys {
		for j := 0; j < numBlocksPerKey; j++ {
			block := testBlockEntry(j)
			require.True(t, q.stagedBlocks(key).push(block))
			blocks[key] = append(blocks[key], block.id)
		}
	}

	for key, s := range blocks {
		rand.Shuffle(len(s), func(i, j int) {
			s[i], s[j] = s[j], s[i]
		})
		for _, b := range s {
			staged, ok := q.staged[key]
			if !ok {
				return
			}
			assert.NotEmpty(t, staged.delete(b))
		}
	}

	for key := range blocks {
		require.Nil(t, q.staged[key])
	}

	assert.Nil(t, q.head)
	assert.Nil(t, q.tail)
}

func TestBlockQueue_FlushByAge(t *testing.T) {
	s := Config{
		Levels: []LevelConfig{
			{MaxBlocks: 3, MaxAge: 1},
			{MaxBlocks: 5, MaxAge: 1},
		},
	}

	c := newCompactionQueue(s, nil)
	blocks := []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 1, Index: 1, AppendedAt: 5, ID: "1"},
		{Tenant: "A", Shard: 1, Level: 1, Index: 2, AppendedAt: 15, ID: "2"},
		{Tenant: "A", Shard: 0, Level: 1, Index: 3, AppendedAt: 30, ID: "3"},
		{Tenant: "A", Shard: 0, Level: 1, Index: 4, AppendedAt: 35, ID: "4"},
		{Tenant: "B", Shard: 0, Level: 1, Index: 5, AppendedAt: 40, ID: "5"},
	}
	for _, e := range blocks {
		c.push(e)
	}

	batches := make([]blockEntry, 0, len(blocks))
	q := c.blockQueue(1)
	iter := newBatchIter(q)
	for {
		b, ok := iter.next()
		if !ok {
			break
		}
		batches = append(batches, b.blocks...)
	}

	expected := []blockEntry{{"1", 1}, {"2", 2}, {"3", 3}, {"4", 4}}
	// "5" remains staged as we need another push to evict it.
	assert.Equal(t, expected, batches)

	// We have 3 compaction queues (staging batches):
	// A/1/1, A/0/1, B/0/1.
	assert.Equal(t, 3, q.updates.Len())

	staged := q.stagedBlocks(compactionKey{tenant: "B", shard: 0, level: 1})
	staged.delete("5")
	// B/0/1 compaction queue is removed.
	assert.Equal(t, 2, q.updates.Len())

	staged = q.stagedBlocks(compactionKey{tenant: "A", shard: 0, level: 1})
	staged.delete("4")
	// A/0/1 is still here.
	assert.Equal(t, 2, q.updates.Len())
	staged.delete("3")
	// A/0/1 compaction queue is removed.
	assert.Equal(t, 1, q.updates.Len())

	// A/1/1 compaction queue is removed.
	staged = q.stagedBlocks(compactionKey{tenant: "A", shard: 1, level: 1})
	assert.NotEmpty(t, staged.delete("1"))
	assert.Equal(t, 1, q.updates.Len())
	assert.NotEmpty(t, staged.delete("2"))
	assert.Equal(t, 0, staged.queue.updates.Len())
}

func TestBlockQueue_BatchIterator(t *testing.T) {
	q := testBlockQueue(Config{Levels: []LevelConfig{{MaxBlocks: 3}}})
	keys := []compactionKey{
		{tenant: "t-1", shard: 1},
		{tenant: "t-2", shard: 2},
	}

	for j := 0; j < 20; j++ {
		q.stagedBlocks(keys[j%len(keys)]).push(testBlockEntry(j))
	}

	iter := newBatchIter(q)
	for _, expected := range []struct {
		key    compactionKey
		blocks []string
	}{
		{key: keys[0], blocks: []string{"0", "2", "4"}},
		{key: keys[1], blocks: []string{"1", "3", "5"}},
		{key: keys[0], blocks: []string{"6", "8", "10"}},
		{key: keys[1], blocks: []string{"7", "9", "11"}},
		{key: keys[0], blocks: []string{"12", "14", "16"}},
		{key: keys[1], blocks: []string{"13", "15", "17"}},
	} {
		b, ok := iter.next()
		require.True(t, ok)
		assert.Equal(t, expected.key, b.staged.key)
		actual := make([]string, len(b.blocks))
		for i := range b.blocks {
			actual[i] = b.blocks[i].id
		}
		assert.Equal(t, expected.blocks, actual)
	}

	_, ok := iter.next()
	assert.False(t, ok)
}

func remove(q *blockQueue, key compactionKey, block ...string) {
	staged, ok := q.staged[key]
	if !ok {
		return
	}
	for _, b := range block {
		staged.delete(b)
	}
}
