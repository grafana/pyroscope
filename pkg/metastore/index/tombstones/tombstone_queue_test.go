package tombstones

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/index/tombstones/store"
)

func TestTombstoneIterator(t *testing.T) {
	queue := newTombstoneQueue()
	now := time.Now()
	entries := []*tombstones{
		{
			TombstoneEntry: store.TombstoneEntry{
				Index:      1,
				AppendedAt: now.Add(-5 * time.Hour).UnixNano(),
				Tombstones: &metastorev1.Tombstones{
					Blocks: &metastorev1.BlockTombstones{Name: "block-1"},
				},
			},
		},
		{
			TombstoneEntry: store.TombstoneEntry{
				Index:      2,
				AppendedAt: now.Add(-4 * time.Hour).UnixNano(),
				Tombstones: &metastorev1.Tombstones{
					Blocks: &metastorev1.BlockTombstones{Name: "block-2"},
				},
			},
		},
		{
			TombstoneEntry: store.TombstoneEntry{
				Index:      3,
				AppendedAt: now.Add(-3 * time.Hour).UnixNano(),
				Tombstones: &metastorev1.Tombstones{
					Blocks: &metastorev1.BlockTombstones{Name: "block-3"},
				},
			},
		},
		{
			TombstoneEntry: store.TombstoneEntry{
				Index:      4,
				AppendedAt: now.Add(-2 * time.Hour).UnixNano(),
				Tombstones: &metastorev1.Tombstones{
					Blocks: &metastorev1.BlockTombstones{Name: "block-4"},
				},
			},
		},
		{
			TombstoneEntry: store.TombstoneEntry{
				Index:      5,
				AppendedAt: now.Add(-1 * time.Hour).UnixNano(),
				Tombstones: &metastorev1.Tombstones{
					Blocks: &metastorev1.BlockTombstones{Name: "block-5"},
				},
			},
		},
	}

	for _, entry := range entries {
		queue.push(entry)
	}

	t.Run("all entries before current time", func(t *testing.T) {
		iter := &tombstoneIter{
			head:   queue.head,
			before: now.UnixNano(),
		}
		count := 0
		for iter.Next() {
			count++
			assert.Equal(t, entries[count-1].Tombstones, iter.At())
		}
		assert.Equal(t, len(entries), count)
	})

	t.Run("entries before specific time", func(t *testing.T) {
		cutoffTime := now.Add(-3 * time.Hour)
		iter := &tombstoneIter{
			head:   queue.head,
			before: cutoffTime.UnixNano(),
		}
		expected := []string{"block-1", "block-2"}
		var actual []string
		for iter.Next() {
			actual = append(actual, iter.At().Blocks.Name)
		}
		assert.Equal(t, expected, actual)
	})

	t.Run("empty queue", func(t *testing.T) {
		emptyQueue := newTombstoneQueue()
		iter := &tombstoneIter{
			head:   emptyQueue.head,
			before: now.UnixNano(),
		}
		assert.False(t, iter.Next())
	})

	t.Run("no entries before cutoff time", func(t *testing.T) {
		iter := &tombstoneIter{
			head:   queue.head,
			before: now.Add(-10 * time.Hour).UnixNano(),
		}
		assert.False(t, iter.Next())
	})
}

func TestQueueDelete(t *testing.T) {
	queue := newTombstoneQueue()
	now := time.Now()

	entry1 := &tombstones{
		TombstoneEntry: store.TombstoneEntry{
			Index:      1,
			AppendedAt: now.Add(-3 * time.Hour).UnixNano(),
			Tombstones: &metastorev1.Tombstones{
				Blocks: &metastorev1.BlockTombstones{Name: "tombstone-1"},
			},
		},
	}
	entry2 := &tombstones{
		TombstoneEntry: store.TombstoneEntry{
			Index:      2,
			AppendedAt: now.Add(-2 * time.Hour).UnixNano(),
			Tombstones: &metastorev1.Tombstones{
				Blocks: &metastorev1.BlockTombstones{Name: "tombstone-2"},
			},
		},
	}
	entry3 := &tombstones{
		TombstoneEntry: store.TombstoneEntry{
			Index:      3,
			AppendedAt: now.Add(-1 * time.Hour).UnixNano(),
			Tombstones: &metastorev1.Tombstones{
				Blocks: &metastorev1.BlockTombstones{Name: "tombstone-3"},
			},
		},
	}

	queue.push(entry1)
	queue.push(entry2)
	queue.push(entry3)

	assert.Equal(t, entry1, queue.head)
	assert.Equal(t, entry3, queue.tail)

	deleted := queue.delete(entry3)
	assert.Equal(t, entry3, deleted)

	assert.NotNil(t, queue.tail)
	assert.Equal(t, entry2, queue.tail)

	var count int
	iter := &tombstoneIter{head: queue.head, before: now.UnixNano()}
	for iter.Next() {
		count++
	}

	assert.Equal(t, 2, count)
}
