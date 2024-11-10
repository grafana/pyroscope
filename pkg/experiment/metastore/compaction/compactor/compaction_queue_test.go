package compactor

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBlockEntry(id int) blockEntry { return blockEntry{id: strconv.Itoa(id)} }

func TestBlockQueue_Push(t *testing.T) {
	q := newBlockQueue(Strategy{MaxBlocksDefault: 3})
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

	q.remove(key, "1", "2") // Remove the first batch.
	assert.Equal(t, []blockEntry{zeroBlockEntry, zeroBlockEntry, testBlockEntry(3)}, q.head.blocks)
	q.remove(key, "3")
	assert.Nil(t, q.head)

	q.stagedBlocks(key).push(testBlockEntry(6)) // Complete the second batch.
	assert.Equal(t, 0, len(q.staged[key].batch.blocks))

	q.stagedBlocks(key).push(testBlockEntry(7))
	assert.Equal(t, []blockEntry{testBlockEntry(4), testBlockEntry(5), testBlockEntry(6)}, q.head.blocks)
	assert.Equal(t, 1, len(q.staged[key].batch.blocks))
}

func TestBlockQueue_DuplicateBlock(t *testing.T) {
	q := newBlockQueue(Strategy{MaxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}

	require.True(t, q.stagedBlocks(key).push(testBlockEntry(1)))
	require.False(t, q.stagedBlocks(key).push(testBlockEntry(1)))

	assert.Equal(t, 1, len(q.staged[key].batch.blocks))
}

func TestBlockQueue_Remove(t *testing.T) {
	q := newBlockQueue(Strategy{MaxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}
	q.stagedBlocks(key).push(testBlockEntry(1))
	q.stagedBlocks(key).push(testBlockEntry(2))

	q.remove(key, "1")
	require.Empty(t, q.staged[key].batch.blocks[0])

	_, exists := q.staged[key].refs["1"]
	assert.False(t, exists)

	q.remove(key, "2")
	require.Nil(t, q.head)
	require.Nil(t, q.tail)
}

func TestBlockQueue_RemoveNotFound(t *testing.T) {
	q := newBlockQueue(Strategy{MaxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}
	q.remove(key, "1")
	q.stagedBlocks(key).push(testBlockEntry(1))
	q.remove(key, "2")
	q.stagedBlocks(key).push(testBlockEntry(2))
	q.stagedBlocks(key).push(testBlockEntry(3))

	assert.Equal(t, []blockEntry{testBlockEntry(1), testBlockEntry(2), testBlockEntry(3)}, q.head.blocks)
}

func TestBlockQueue_Linking(t *testing.T) {
	q := newBlockQueue(Strategy{MaxBlocksDefault: 2})
	key := compactionKey{tenant: "t", shard: 1}

	q.stagedBlocks(key).push(testBlockEntry(1))
	q.stagedBlocks(key).push(testBlockEntry(2))
	require.NotNil(t, q.head)
	assert.Equal(t, q.head, q.tail)

	q.stagedBlocks(key).push(testBlockEntry(3))
	assert.NotNil(t, q.tail)
	assert.Nil(t, q.tail.prevG)
	assert.NotNil(t, q.head)
	assert.Nil(t, q.head.nextG)
	assert.Equal(t, []blockEntry{testBlockEntry(1), testBlockEntry(2)}, q.head.blocks)
	assert.Equal(t, q.tail.blocks, q.head.blocks)

	q.stagedBlocks(key).push(testBlockEntry(4))
	assert.NotNil(t, q.tail.prevG)
	assert.NotNil(t, q.head.nextG)

	q.stagedBlocks(key).push(testBlockEntry(5))
	q.stagedBlocks(key).push(testBlockEntry(6))
	assert.NotNil(t, q.tail.prevG.prevG)
	assert.NotNil(t, q.head.nextG.nextG)

	q.remove(key, "3", "2")
	q.remove(key, "4", "1")
	q.remove(key, "6")
	q.remove(key, "5")

	assert.Nil(t, q.head)
	assert.Nil(t, q.tail)
}

func TestBlockQueue_ExpectEmptyQueue(t *testing.T) {
	const (
		numKeys         = 5
		numBlocksPerKey = 10
	)

	q := newBlockQueue(Strategy{MaxBlocksDefault: 3})
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
			q.remove(key, b)
		}
	}

	for key := range blocks {
		staged, exists := q.staged[key]
		require.True(t, exists)
		for _, block := range staged.batch.blocks {
			assert.Equal(t, zeroBlockEntry, block)
		}
	}

	assert.Nil(t, q.head)
	assert.Nil(t, q.tail)
}

func TestBlockQueue_FlushByAge(t *testing.T) {
	c := newCompactionQueue(Strategy{
		MaxBlocksDefault: 5,
		MaxBatchAge:      1,
	})

	for _, e := range []BlockEntry{
		{Tenant: "A", Shard: 1, Level: 1, Index: 1, AppendedAt: 5, ID: "1"},
		{Tenant: "A", Shard: 1, Level: 1, Index: 2, AppendedAt: 15, ID: "2"},
		{Tenant: "A", Shard: 0, Level: 1, Index: 3, AppendedAt: 30, ID: "3"},
	} {
		c.push(e)
	}

	batches := make([]blockEntry, 0, 3)
	iter := newBatchIter(c.blockQueue(1))
	for {
		b, ok := iter.next()
		if !ok {
			break
		}
		batches = append(batches, b.blocks...)
	}

	expected := []blockEntry{{"1", 1}, {"2", 2}}
	assert.Equal(t, expected, batches)
}

func TestBlockQueue_BatchIterator(t *testing.T) {
	q := newBlockQueue(Strategy{MaxBlocksDefault: 3})
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
