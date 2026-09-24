package store

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

// ListEntryStats must agree with ListEntries on everything it reports.
func TestBlockQueueStore_ListEntryStats(t *testing.T) {
	db := test.BoltDB(t)
	s := NewBlockQueueStore()

	entries := []compaction.BlockEntry{
		{Index: 1, AppendedAt: 100, ID: "b1", Tenant: "tenant-a", Shard: 1, Level: 0},
		{Index: 2, AppendedAt: 200, ID: "b2", Tenant: "", Shard: 2, Level: 0},
		{Index: 3, AppendedAt: 300, ID: "b3", Tenant: "tenant-a", Shard: 1, Level: 1},
	}
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, s.CreateBuckets(tx))
		for _, e := range entries {
			require.NoError(t, s.StoreEntry(tx, e))
		}
		return nil
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		it := s.ListEntryStats(tx)
		defer func() { _ = it.Close() }()
		var got []compaction.BlockEntryStats
		for it.Next() {
			got = append(got, it.At())
		}
		require.NoError(t, it.Err())
		assert.Equal(t, []compaction.BlockEntryStats{
			{AppendedAt: 100, Tenant: "tenant-a", Shard: 1, Level: 0},
			{AppendedAt: 200, Tenant: "", Shard: 2, Level: 0},
			{AppendedAt: 300, Tenant: "tenant-a", Shard: 1, Level: 1},
		}, got)
		return nil
	}))
}

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
