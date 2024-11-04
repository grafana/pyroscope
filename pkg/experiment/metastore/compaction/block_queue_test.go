package compaction

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockQueue_Push(t *testing.T) {
	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}

	result := q.push(key, "1")
	require.True(t, result)
	require.Equal(t, 1, len(q.staged[key].batch.blocks))
	assert.Equal(t, "1", q.staged[key].batch.blocks[0])

	q.push(key, "2")
	q.push(key, "3") // Staged blocks formed the first batch.
	assert.Equal(t, 0, len(q.staged[key].batch.blocks))
	assert.Equal(t, []string{"1", "2", "3"}, q.head.blocks)

	q.push(key, "4")
	q.push(key, "5")
	assert.Equal(t, 2, len(q.staged[key].batch.blocks))

	q.remove(key, "1", "2") // Remove the first batch.
	assert.Equal(t, []string{"", "", "3"}, q.head.blocks)
	q.remove(key, "3")
	assert.Nil(t, q.head)

	q.push(key, "6") // Complete the second batch.
	assert.Equal(t, 0, len(q.staged[key].batch.blocks))

	q.push(key, "7")
	assert.Equal(t, []string{"4", "5", "6"}, q.head.blocks)
	assert.Equal(t, 1, len(q.staged[key].batch.blocks))
}

func TestBlockQueue_DuplicateBlock(t *testing.T) {
	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}
	block := "1"

	require.True(t, q.push(key, block))
	require.False(t, q.push(key, block))

	assert.Equal(t, 1, len(q.staged[key].batch.blocks))
}

func TestBlockQueue_Remove(t *testing.T) {
	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}
	q.push(key, "1")
	q.push(key, "2")

	q.remove(key, "1")
	require.Equal(t, "", q.staged[key].batch.blocks[0])

	_, exists := q.staged[key].refs["1"]
	assert.False(t, exists)

	q.remove(key, "2")
	require.Nil(t, q.head)
	require.Nil(t, q.tail)
}

func TestBlockQueue_Remove_not_found(t *testing.T) {
	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 3})
	key := compactionKey{tenant: "t", shard: 1}
	q.remove(key, "1")
	q.push(key, "1")
	q.remove(key, "2")
	q.push(key, "2")
	q.push(key, "3")

	assert.Equal(t, []string{"1", "2", "3"}, q.head.blocks)
}

func TestBlockQueue_Linking(t *testing.T) {
	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 2})
	key := compactionKey{tenant: "t", shard: 1}

	q.push(key, "1")
	q.push(key, "2")
	require.NotNil(t, q.head)
	assert.Equal(t, q.head, q.tail)

	q.push(key, "3")
	assert.NotNil(t, q.tail)
	assert.Nil(t, q.tail.prev)
	assert.NotNil(t, q.head)
	assert.Nil(t, q.head.next)
	assert.Equal(t, []string{"1", "2"}, q.head.blocks)
	assert.Equal(t, q.tail.blocks, q.head.blocks)

	q.push(key, "4")
	assert.NotNil(t, q.tail.prev)
	assert.NotNil(t, q.head.next)

	q.push(key, "5")
	q.push(key, "6")
	assert.NotNil(t, q.tail.prev.prev)
	assert.NotNil(t, q.head.next.next)

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

	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 3})
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
			block := fmt.Sprint(j)
			require.True(t, q.push(key, block))
			blocks[key] = append(blocks[key], block)
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
			assert.Equal(t, removedBlockSentinel, block)
		}
	}

	assert.Nil(t, q.head)
	assert.Nil(t, q.tail)
}

func TestBlockQueue_iter(t *testing.T) {
	q := newBlockQueue(jobSizeCompactionStrategy{maxBlocksDefault: 3})
	keys := []compactionKey{
		{tenant: "t-1", shard: 1},
		{tenant: "t-2", shard: 2},
	}

	for j := 0; j < 20; j++ {
		q.push(keys[j%len(keys)], strconv.Itoa(j))
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
		assert.Equal(t, expected.key, b.staged.compactionKey)
		assert.Equal(t, expected.blocks, b.blocks)
	}

	_, ok := iter.next()
	assert.False(t, ok)
}
