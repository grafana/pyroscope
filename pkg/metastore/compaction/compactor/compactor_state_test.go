package compactor

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor/store"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

func TestCompactor_ListQueues(t *testing.T) {
	db := test.BoltDB(t)
	s := store.NewBlockQueueStore()
	compactor := NewCompactor(DefaultConfig(), s, nil, nil)

	entries := []compaction.BlockEntry{
		{Index: 1, ID: "b1", Tenant: "tenant-a", Shard: 1, Level: 0, AppendedAt: 100},
		{Index: 2, ID: "b2", Tenant: "tenant-a", Shard: 1, Level: 0, AppendedAt: 300},
		{Index: 3, ID: "b3", Tenant: "tenant-a", Shard: 1, Level: 0, AppendedAt: 200},
		{Index: 4, ID: "b4", Tenant: "tenant-a", Shard: 2, Level: 0, AppendedAt: 400},
		{Index: 5, ID: "b5", Tenant: "tenant-b", Shard: 1, Level: 1, AppendedAt: 500},
	}
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, s.CreateBuckets(tx))
		for _, e := range entries {
			require.NoError(t, s.StoreEntry(tx, e))
		}
		return nil
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		queues, _, err := compactor.ListQueues(context.Background(), tx, QueueFilter{})
		require.NoError(t, err)
		assert.Equal(t, []QueueStats{
			{Tenant: "tenant-a", Shard: 1, Level: 0, Blocks: 3, OldestAppendedAt: 100, NewestAppendedAt: 300},
			{Tenant: "tenant-a", Shard: 2, Level: 0, Blocks: 1, OldestAppendedAt: 400, NewestAppendedAt: 400},
			{Tenant: "tenant-b", Shard: 1, Level: 1, Blocks: 1, OldestAppendedAt: 500, NewestAppendedAt: 500},
		}, queues)
		return nil
	}))
}

func TestCompactor_ListQueues_canceled(t *testing.T) {
	db := test.BoltDB(t)
	s := store.NewBlockQueueStore()
	compactor := NewCompactor(DefaultConfig(), s, nil, nil)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, s.CreateBuckets(tx))
		for i := 0; i < 5000; i++ {
			require.NoError(t, s.StoreEntry(tx, compaction.BlockEntry{
				Index: uint64(i), ID: strconv.Itoa(i), Tenant: "tenant-a", AppendedAt: int64(i),
			}))
		}
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		_, _, err := compactor.ListQueues(ctx, tx, QueueFilter{})
		require.ErrorIs(t, err, context.Canceled)
		return nil
	}))
}

func TestCompactor_ListQueues_filter(t *testing.T) {
	db := test.BoltDB(t)
	s := store.NewBlockQueueStore()
	compactor := NewCompactor(DefaultConfig(), s, nil, nil)

	// Level 0 entries have no tenant: they hold the multi-tenant segments.
	entries := []compaction.BlockEntry{
		{Index: 1, AppendedAt: 100, ID: "b1", Tenant: "tenant-a", Shard: 1, Level: 1},
		{Index: 2, AppendedAt: 200, ID: "b2", Tenant: "", Shard: 1, Level: 0},
		{Index: 3, AppendedAt: 300, ID: "b3", Tenant: "tenant-b", Shard: 1, Level: 1},
	}
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, s.CreateBuckets(tx))
		for _, e := range entries {
			require.NoError(t, s.StoreEntry(tx, e))
		}
		return nil
	}))

	tenants := func(t *testing.T, filter QueueFilter) []string {
		var found []string
		require.NoError(t, db.View(func(tx *bbolt.Tx) error {
			queues, _, err := compactor.ListQueues(context.Background(), tx, filter)
			require.NoError(t, err)
			for _, q := range queues {
				found = append(found, q.Tenant)
			}
			return nil
		}))
		return found
	}

	tenantA := "tenant-a"
	noTenant := ""

	assert.Equal(t, []string{"", "tenant-a", "tenant-b"}, tenants(t, QueueFilter{}))
	assert.Equal(t, []string{"tenant-a"}, tenants(t, QueueFilter{Tenant: &tenantA}))
	assert.Equal(t, []string{""}, tenants(t, QueueFilter{Tenant: &noTenant}))
}

func TestCompactor_ListQueues_truncated(t *testing.T) {
	db := test.BoltDB(t)
	s := store.NewBlockQueueStore()
	compactor := NewCompactor(DefaultConfig(), s, nil, nil)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, s.CreateBuckets(tx))
		for i := 0; i < 10; i++ {
			require.NoError(t, s.StoreEntry(tx, compaction.BlockEntry{
				Index: uint64(i), ID: strconv.Itoa(i), Tenant: "tenant-a", AppendedAt: int64(i),
			}))
		}
		return nil
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		queues, truncated, err := compactor.ListQueues(context.Background(), tx, QueueFilter{MaxEntries: 4})
		require.NoError(t, err)
		assert.True(t, truncated)
		// The statistics cover only the entries that were read.
		require.Len(t, queues, 1)
		assert.Equal(t, uint64(4), queues[0].Blocks)

		// A cap that the queue does not reach is not a truncation.
		queues, truncated, err = compactor.ListQueues(context.Background(), tx, QueueFilter{MaxEntries: 10})
		require.NoError(t, err)
		assert.False(t, truncated)
		require.Len(t, queues, 1)
		assert.Equal(t, uint64(10), queues[0].Blocks)
		return nil
	}))
}

func TestCompactor_ListQueues_empty(t *testing.T) {
	db := test.BoltDB(t)
	s := store.NewBlockQueueStore()
	compactor := NewCompactor(DefaultConfig(), s, nil, nil)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return s.CreateBuckets(tx)
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		queues, _, err := compactor.ListQueues(context.Background(), tx, QueueFilter{})
		require.NoError(t, err)
		assert.Empty(t, queues)
		return nil
	}))
}
