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

func TestTombstonesIdempotence(t *testing.T) {
	db := test.BoltDB(t)
	tombstoneStore := store.NewTombstoneStore()

	ts := NewTombstones(tombstoneStore, nil)
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, ts.Init(tx))
	require.NoError(t, tx.Commit())

	now := time.Now()
	cmd := &raft.Log{
		Index:      1,
		Term:       1,
		Type:       raft.LogCommand,
		Data:       []byte("test"),
		AppendedAt: now,
	}

	x := &metastorev1.Tombstones{
		Blocks: &metastorev1.BlockTombstones{
			Name:   "test-block",
			Tenant: "test-tenant",
			Shard:  1,
			Blocks: []string{"block-1", "block-2"},
		},
	}

	t.Run("AddTombstones", func(t *testing.T) {
		tx, err = db.Begin(true)
		require.NoError(t, err)
		err = ts.AddTombstones(tx, cmd, x)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		assert.True(t, ts.Exists("test-tenant", 1, "block-1"))
		assert.True(t, ts.Exists("test-tenant", 1, "block-2"))

		count := countTombstones(ts)

		tx, err = db.Begin(true)
		require.NoError(t, err)
		err = ts.AddTombstones(tx, cmd, x)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		assert.Equal(t, count, countTombstones(ts))

		assert.True(t, ts.Exists("test-tenant", 1, "block-1"))
		assert.True(t, ts.Exists("test-tenant", 1, "block-2"))
	})

	t.Run("DeleteTombstones", func(t *testing.T) {
		tx, err := db.Begin(true)
		require.NoError(t, err)
		err = ts.DeleteTombstones(tx, cmd, x)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		assert.False(t, ts.Exists("test-tenant", 1, "block-1"))
		assert.False(t, ts.Exists("test-tenant", 1, "block-2"))

		count := countTombstones(ts)

		tx, err = db.Begin(true)
		require.NoError(t, err)
		err = ts.DeleteTombstones(tx, cmd, x)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		assert.Equal(t, count, countTombstones(ts))

		assert.False(t, ts.Exists("test-tenant", 1, "block-1"))
		assert.False(t, ts.Exists("test-tenant", 1, "block-2"))
	})
}

func countTombstones(ts *Tombstones) int {
	var c int
	iter := ts.ListTombstones(time.Now().Add(time.Hour))
	for iter.Next() {
		c++
	}
	return c
}
