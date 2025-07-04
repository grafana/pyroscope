package tombstones

import (
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/index/tombstones/store"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestTombstonesRestore(t *testing.T) {
	now := time.Now()
	db := test.BoltDB(t)
	tombstoneStore := store.NewTombstoneStore()

	ts := NewTombstones(tombstoneStore, nil)
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, ts.Init(tx))
	require.NoError(t, tx.Commit())

	for _, tombstone := range []*metastorev1.Tombstones{
		{
			Blocks: &metastorev1.BlockTombstones{
				Name:   "x-1",
				Tenant: "tenant-1",
				Shard:  1,
				Blocks: []string{"block-1-1", "block-1-2"},
			},
		},
		{
			Blocks: &metastorev1.BlockTombstones{
				Name:   "x-2",
				Tenant: "tenant-1",
				Shard:  1,
				Blocks: []string{"block-2-1", "block-2-2"},
			},
		},
		{
			Blocks: &metastorev1.BlockTombstones{
				Name:   "x-3",
				Tenant: "tenant-2",
				Shard:  2,
				Blocks: []string{"block-3-1", "block-3-2"},
			},
		},
	} {
		tx, err := db.Begin(true)
		require.NoError(t, err)
		err = ts.AddTombstones(tx, &raft.Log{Index: 1, AppendedAt: now}, tombstone)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	}

	assert.True(t, ts.Exists("tenant-1", 1, "block-1-1"))
	assert.True(t, ts.Exists("tenant-1", 1, "block-1-2"))
	assert.True(t, ts.Exists("tenant-1", 1, "block-2-1"))
	assert.True(t, ts.Exists("tenant-1", 1, "block-2-2"))
	assert.True(t, ts.Exists("tenant-2", 2, "block-3-1"))
	assert.True(t, ts.Exists("tenant-2", 2, "block-3-2"))
	assert.Equal(t, 3, countTombstones(ts))

	restored := NewTombstones(tombstoneStore, nil)
	tx, err = db.Begin(true)
	require.NoError(t, err)
	err = restored.Restore(tx)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	assert.True(t, restored.Exists("tenant-1", 1, "block-1-1"))
	assert.True(t, restored.Exists("tenant-1", 1, "block-1-2"))
	assert.True(t, restored.Exists("tenant-1", 1, "block-2-1"))
	assert.True(t, restored.Exists("tenant-1", 1, "block-2-2"))
	assert.True(t, restored.Exists("tenant-2", 2, "block-3-1"))
	assert.True(t, restored.Exists("tenant-2", 2, "block-3-2"))
	assert.Equal(t, 3, countTombstones(restored))

	futureTime := now.Add(time.Hour)
	iter := restored.ListTombstones(futureTime)
	count := 0
	for iter.Next() {
		count++
	}
	assert.Equal(t, 3, count)
}
