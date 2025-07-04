package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestBlockQueueStore_StoreEntry(t *testing.T) {
	db := test.BoltDB(t)

	s := NewTombstoneStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entries := make([]TombstoneEntry, 1000)
	for i := range entries {
		entries[i] = TombstoneEntry{
			Index:      uint64(i),
			AppendedAt: time.Now().UnixNano(),
			Tombstones: &metastorev1.Tombstones{
				Blocks: &metastorev1.BlockTombstones{Name: "a"},
			},
		}
	}
	for i := range entries {
		assert.NoError(t, s.StoreTombstones(tx, entries[i]))
	}
	require.NoError(t, tx.Commit())

	s = NewTombstoneStore()
	tx, err = db.Begin(false)
	require.NoError(t, err)
	iter := s.ListEntries(tx)
	var i int
	for iter.Next() {
		assert.Less(t, i, len(entries))
		actual := iter.At()
		expected := entries[i]
		assert.Equal(t, expected.Index, actual.Index)
		assert.Equal(t, expected.AppendedAt, actual.AppendedAt)
		assert.Equal(t, expected.Tombstones, actual.Tombstones)
		assert.NotNil(t, actual.key)
		i++
	}
	assert.Nil(t, iter.Err())
	assert.Nil(t, iter.Close())
	require.NoError(t, tx.Rollback())
}

func TestTombstoneStore_DeleteQueuedEntries(t *testing.T) {
	db := test.BoltDB(t)

	s := NewTombstoneStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))

	entries := make([]TombstoneEntry, 1000)
	for i := range entries {
		entries[i] = TombstoneEntry{
			Index:      uint64(i),
			AppendedAt: time.Now().UnixNano(),
			Tombstones: &metastorev1.Tombstones{
				Blocks: &metastorev1.BlockTombstones{Name: "a"},
			},
		}
	}
	for i := range entries {
		assert.NoError(t, s.StoreTombstones(tx, entries[i]))
	}
	require.NoError(t, tx.Commit())

	// Delete random 25%.
	tx, err = db.Begin(true)
	require.NoError(t, err)
	for i := 0; i < len(entries); i += 4 {
		assert.NoError(t, s.DeleteTombstones(tx, entries[i]))
	}
	require.NoError(t, tx.Commit())

	// Check remaining entries.
	s = NewTombstoneStore()
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
		actual := iter.At()
		expected := entries[i]
		assert.Equal(t, expected.Index, actual.Index)
		assert.Equal(t, expected.AppendedAt, actual.AppendedAt)
		assert.Equal(t, expected.Tombstones, actual.Tombstones)
		assert.NotNil(t, actual.key)
		i++
	}
	assert.Nil(t, iter.Err())
	assert.Nil(t, iter.Close())
	require.NoError(t, tx.Rollback())
}
