package store

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestBlockQueueStore_StoreEntry(t *testing.T) {
	db := test.BoltDB(t)

	s := NewBlockQueueStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entries := make([]compaction.BlockEntry, 1000)
	for i := range entries {
		entries[i] = compaction.BlockEntry{
			Index:      uint64(i),
			ID:         strconv.Itoa(i),
			AppendedAt: time.Now().UnixNano(),
			Level:      uint32(i % 3),
			Shard:      uint32(i % 8),
			Tenant:     strconv.Itoa(i % 4),
		}
	}
	for i := range entries {
		assert.NoError(t, s.StoreEntry(tx, entries[i]))
	}
	require.NoError(t, tx.Commit())

	s = NewBlockQueueStore()
	tx, err = db.Begin(false)
	require.NoError(t, err)
	iter := s.ListEntries(tx)
	var i int
	for iter.Next() {
		assert.Less(t, i, len(entries))
		assert.Equal(t, entries[i], iter.At())
		i++
	}
	assert.Nil(t, iter.Err())
	assert.Nil(t, iter.Close())
	require.NoError(t, tx.Rollback())
}

func TestBlockQueueStore_DeleteEntry(t *testing.T) {
	db := test.BoltDB(t)

	s := NewBlockQueueStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entries := make([]compaction.BlockEntry, 1000)
	for i := range entries {
		entries[i] = compaction.BlockEntry{
			Index:      uint64(i),
			ID:         strconv.Itoa(i),
			AppendedAt: time.Now().UnixNano(),
			Level:      uint32(i % 3),
			Shard:      uint32(i % 8),
			Tenant:     strconv.Itoa(i % 4),
		}
	}
	for i := range entries {
		assert.NoError(t, s.StoreEntry(tx, entries[i]))
	}
	require.NoError(t, tx.Commit())

	// Delete random 25%.
	tx, err = db.Begin(true)
	require.NoError(t, err)
	for i := 0; i < len(entries); i += 4 {
		assert.NoError(t, s.DeleteEntry(tx, entries[i].Index, entries[i].ID))
	}
	require.NoError(t, tx.Commit())

	// Check remaining entries.
	s = NewBlockQueueStore()
	tx, err = db.Begin(false)
	require.NoError(t, err)
	iter := s.ListEntries(tx)
	var i int
	for iter.Next() {
		if i%4 == 0 {
			// Skip deleted entries.
			i++
		}
		assert.Less(t, i, len(entries))
		assert.Equal(t, entries[i], iter.At())
		i++
	}
	assert.Nil(t, iter.Err())
	assert.Nil(t, iter.Close())
	require.NoError(t, tx.Rollback())
}
