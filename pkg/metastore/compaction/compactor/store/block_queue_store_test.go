package store

import (
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/test"
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
			Size:       uint64(i) * 997,
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
			Size:       uint64(i) * 997,
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

func TestBlockQueueStore_DecodeLegacyFormat(t *testing.T) {
	db := test.BoltDB(t)
	s := NewBlockQueueStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	// Hand-construct an entry exactly as the pre-Size marshalBlockEntry produced it:
	// Key = [Index:8][ID], Value = [AppendedAt:8][Level:4][Shard:4][Tenant].
	const (
		index      = uint64(42)
		id         = "01J000000000000000000LEGACY"
		appendedAt = int64(1_700_000_000_000_000_000) // a realistic wall-clock nanosecond timestamp
		level      = uint32(2)
		shard      = uint32(7)
		tenant     = "legacy-tenant"
	)
	key := make([]byte, 8+len(id))
	binary.BigEndian.PutUint64(key, index)
	copy(key[8:], id)

	value := make([]byte, 8+4+4+len(tenant))
	binary.BigEndian.PutUint64(value[0:8], uint64(appendedAt))
	binary.BigEndian.PutUint32(value[8:12], level)
	binary.BigEndian.PutUint32(value[12:16], shard)
	copy(value[16:], tenant)

	require.NoError(t, tx.Bucket(blockQueueBucketName).Put(key, value))
	require.NoError(t, tx.Commit())

	tx, err = db.Begin(false)
	require.NoError(t, err)

	it := s.ListEntries(tx)
	require.True(t, it.Next())
	entry := it.At()
	assert.Equal(t, index, entry.Index)
	assert.Equal(t, id, entry.ID)
	assert.Equal(t, appendedAt, entry.AppendedAt)
	assert.Equal(t, level, entry.Level)
	assert.Equal(t, shard, entry.Shard)
	assert.Equal(t, tenant, entry.Tenant)
	assert.Equal(t, uint64(0), entry.Size) // fallback: legacy entries carry no size data.
	assert.False(t, it.Next())
	require.NoError(t, it.Err())
	require.NoError(t, tx.Rollback())
}

// TestBlockQueueStore_ListEntries_MissingSizeRecord verifies that ListEntries
// joins the main bucket against the size bucket by key, and falls back to
// Size == 0 for an entry whose size record is missing - the same situation a
// downgrade-then-reupgrade, or a pre-Size-tracking legacy entry, produces -
// without affecting the Size of any other, unrelated entry.
func TestBlockQueueStore_ListEntries_MissingSizeRecord(t *testing.T) {
	db := test.BoltDB(t)
	s := NewBlockQueueStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entries := []compaction.BlockEntry{
		{Index: 1, ID: "block-1", Tenant: "A", Level: 1, Shard: 0, Size: 111},
		{Index: 2, ID: "block-2", Tenant: "A", Level: 1, Shard: 0, Size: 222},
	}
	for _, e := range entries {
		require.NoError(t, s.StoreEntry(tx, e))
	}
	// Simulate a missing size record for the first entry (e.g., one
	// written by a binary that never wrote to the size bucket) by
	// deleting only its size record directly, leaving the main entry
	// intact.
	require.NoError(t, tx.Bucket(blockQueueSizeBucketName).Delete(marshalBlockEntryKey(1, "block-1")))
	require.NoError(t, tx.Commit())

	tx, err = db.Begin(false)
	require.NoError(t, err)
	it := s.ListEntries(tx)

	require.True(t, it.Next())
	first := it.At()
	assert.Equal(t, "block-1", first.ID)
	assert.Equal(t, uint64(0), first.Size, "missing size record falls back to 0")

	require.True(t, it.Next())
	second := it.At()
	assert.Equal(t, "block-2", second.ID)
	assert.Equal(t, uint64(222), second.Size, "unrelated entry's size is unaffected")

	require.False(t, it.Next())
	require.NoError(t, it.Err())
	require.NoError(t, tx.Rollback())
}

// TestBlockQueueStore_DeleteEntry_RemovesSizeRecord verifies DeleteEntry
// removes the size record symmetrically with the main entry, so deleted
// entries never accumulate as orphaned records in the size bucket.
func TestBlockQueueStore_DeleteEntry_RemovesSizeRecord(t *testing.T) {
	db := test.BoltDB(t)
	s := NewBlockQueueStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entry := compaction.BlockEntry{Index: 1, ID: "block-1", Tenant: "A", Level: 1, Shard: 0, Size: 555}
	require.NoError(t, s.StoreEntry(tx, entry))
	require.NoError(t, tx.Commit())

	tx, err = db.Begin(true)
	require.NoError(t, err)
	key := marshalBlockEntryKey(entry.Index, entry.ID)
	require.NotNil(t, tx.Bucket(blockQueueSizeBucketName).Get(key), "size record should exist before delete")

	require.NoError(t, s.DeleteEntry(tx, entry.Index, entry.ID))
	assert.Nil(t, tx.Bucket(blockQueueBucketName).Get(key), "main entry should be gone")
	assert.Nil(t, tx.Bucket(blockQueueSizeBucketName).Get(key), "size record should be gone too, not orphaned")
	require.NoError(t, tx.Commit())
}

func TestBlockQueueStore_EmptyTenant(t *testing.T) {
	db := test.BoltDB(t)
	s := NewBlockQueueStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entry := compaction.BlockEntry{
		Index: 1, ID: "block-1", AppendedAt: time.Now().UnixNano(),
		Level: 1, Shard: 0, Tenant: "", Size: 9999,
	}
	require.NoError(t, s.StoreEntry(tx, entry))
	require.NoError(t, tx.Commit())

	tx, err = db.Begin(false)
	require.NoError(t, err)

	it := s.ListEntries(tx)
	require.True(t, it.Next())
	require.Equal(t, entry, it.At())
	require.False(t, it.Next())
	require.NoError(t, tx.Rollback())
}
