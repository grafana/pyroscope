// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

// The blocks are dated well in the past on purpose. Restore only reloads
// partitions overlapping now +/- queryLookaroundPeriod, so an older partition
// is exactly the case where a cache entry survives without being refreshed.
var (
	restoreMinT = test.UnixMilli("2024-09-23T08:00:00.000Z")
	restoreMaxT = test.UnixMilli("2024-09-23T09:00:00.000Z")
	restoreIDA  = test.ULID("2024-09-23T08:00:00.001Z")
	restoreIDB  = test.ULID("2024-09-23T08:30:00.002Z")
	restoreIDC  = test.ULID("2024-09-23T08:45:00.003Z")
)

func restoreTestBlock(id, service string) *metastorev1.BlockMeta {
	return &metastorev1.BlockMeta{
		Id: id, Tenant: 1, Shard: 1, MinTime: restoreMinT, MaxTime: restoreMaxT,
		Datasets: []*metastorev1.Dataset{{
			Tenant: 1, MinTime: restoreMinT, MaxTime: restoreMaxT, Labels: []int32{1, 2, 3},
		}},
		StringTable: []string{"", "tenant-a", "service_name", service},
	}
}

// restoreTestFixture builds the state a follower is in when it installs a
// snapshot: a local index holding block A, and a snapshot database holding
// blocks A and B, so the snapshot's string table is longer than the one the
// local index has cached.
func restoreTestFixture(t *testing.T) (*Index, *bbolt.DB) {
	t.Helper()

	idx := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	local := test.BoltDB(t)
	require.NoError(t, local.Update(idx.Init))
	require.NoError(t, local.Update(func(tx *bbolt.Tx) error {
		return idx.InsertBlock(tx, restoreTestBlock(restoreIDA, "svc-a"))
	}))

	snapshot := test.BoltDB(t)
	writer := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, snapshot.Update(writer.Init))
	require.NoError(t, snapshot.Update(func(tx *bbolt.Tx) error {
		if err := writer.InsertBlock(tx, restoreTestBlock(restoreIDA, "svc-a")); err != nil {
			return err
		}
		return writer.InsertBlock(tx, restoreTestBlock(restoreIDB, "svc-b"))
	}))

	// FSM.Restore replaces the database, then runs the restorers under a read
	// transaction.
	require.NoError(t, snapshot.View(idx.Restore))
	return idx, snapshot
}

// TestIndex_Restore_DiscardsStaleShardCache covers a query served from a
// shard cached before the snapshot was installed. The cached shard carries
// the shorter string table while the blocks come from the new database, and
// dataset labels are indices into that table, so the query used to read past
// the end of it and panic.
func TestIndex_Restore_DiscardsStaleShardCache(t *testing.T) {
	idx, snapshot := restoreTestFixture(t)

	require.NoError(t, snapshot.View(func(tx *bbolt.Tx) error {
		_, err := idx.QueryMetadata(tx, context.Background(), MetadataQuery{
			Expr:      `{service_name=~".+"}`,
			StartTime: time.UnixMilli(restoreMinT),
			EndTime:   time.UnixMilli(restoreMaxT),
			Tenant:    []string{"tenant-a"},
			Labels:    []string{"service_name"},
		})
		return err
	}))
}

// TestIndex_Restore_StaleShardWriteKeepsStringTable covers the write side.
// Shard.Store keys a new string chunk by the pre-import table length, so a
// write through a stale cached shard used to overwrite the chunk already at
// that key with a shorter payload. That corrupts the table on disk without
// any error: labels then resolve to the wrong strings.
func TestIndex_Restore_StaleShardWriteKeepsStringTable(t *testing.T) {
	idx, snapshot := restoreTestFixture(t)

	require.NoError(t, snapshot.Update(func(tx *bbolt.Tx) error {
		return idx.InsertBlock(tx, restoreTestBlock(restoreIDC, "svc-c"))
	}))

	// Read back from disk with a cold index, so only what was persisted counts.
	fresh := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, snapshot.View(func(tx *bbolt.Tx) error {
		metas, err := fresh.GetBlocks(tx, &metastorev1.BlockList{
			Tenant: "tenant-a", Shard: 1,
			Blocks: []string{restoreIDA, restoreIDB, restoreIDC},
		})
		require.NoError(t, err)

		want := map[string]string{
			restoreIDA: "svc-a",
			restoreIDB: "svc-b",
			restoreIDC: "svc-c",
		}
		require.Len(t, metas, len(want))
		for _, m := range metas {
			require.Equal(t, want[m.Id], m.StringTable[m.Datasets[0].Labels[2]],
				"service_name of block %s", m.Id)
		}
		return nil
	}))
}

// recentBlockID returns a ULID placing a block in the partition covering at,
// so that Restore reloads it rather than skipping it.
func recentBlockID(at time.Time) string {
	return ulid.MustNew(ulid.Timestamp(at), rand.Reader).String()
}

// recentBlock builds a block for an existing id.
//
// InsertBlock rewrites the metadata it is given in place, interning the
// string table into the shard, so a block inserted into two databases needs
// a separate value for each. Sharing one would leave the second insert
// working from already-interned indices.
func recentBlock(id string, at time.Time, service string) *metastorev1.BlockMeta {
	start, end := at.UnixMilli(), at.Add(30*time.Minute).UnixMilli()
	return &metastorev1.BlockMeta{
		Id: id, Tenant: 1, Shard: 1, MinTime: start, MaxTime: end,
		Datasets: []*metastorev1.Dataset{{
			Tenant: 1, MinTime: start, MaxTime: end, Labels: []int32{1, 2, 3},
		}},
		StringTable: []string{"", "tenant-a", "service_name", service},
	}
}

// TestIndex_Restore_PreloadedShardIsReadOnly covers the other half of Restore:
// a partition inside the lookaround window, which Restore actually reloads.
//
// The preceding tests use partitions outside that window, so they exercise
// the purge but never the preload. Here the shard is loaded by Restore
// itself, under a read transaction, and the question is whether the entry it
// leaves behind is usable by a later write.
func TestIndex_Restore_PreloadedShardIsReadOnly(t *testing.T) {
	// Anchor every block to the start of the current partition. Spacing them
	// by wall-clock offsets instead would straddle a partition boundary
	// whenever the test ran near one.
	now := time.Now()
	base := now.Truncate(DefaultConfig.partitionDuration).Add(time.Minute)
	idA := recentBlockID(base)
	idB := recentBlockID(base.Add(time.Minute))
	idC := recentBlockID(base.Add(2 * time.Minute))

	idx := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	local := test.BoltDB(t)
	require.NoError(t, local.Update(idx.Init))
	require.NoError(t, local.Update(func(tx *bbolt.Tx) error {
		return idx.InsertBlock(tx, recentBlock(idA, base, "svc-a"))
	}))

	snapshot := test.BoltDB(t)
	writer := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, snapshot.Update(writer.Init))
	require.NoError(t, snapshot.Update(func(tx *bbolt.Tx) error {
		if err := writer.InsertBlock(tx, recentBlock(idA, base, "svc-a")); err != nil {
			return err
		}
		return writer.InsertBlock(tx, recentBlock(idB, base.Add(time.Minute), "svc-b"))
	}))

	require.NoError(t, snapshot.View(idx.Restore))

	// The partition is in the window, so Restore loaded the shard rather than
	// skipping it, and it must be marked read-only: it came from a read
	// transaction, and a write that reused it would be building on state the
	// write itself did not load.
	key := shardCacheKey{
		partition: idx.partitionKeyForBlock(idA),
		tenant:    "tenant-a",
		shard:     1,
	}
	cached, found := idx.shards.cache.Get(key)
	require.True(t, found, "Restore should have preloaded the shard for an in-window partition")
	require.True(t, cached.readOnly, "a shard loaded under a read transaction must be cached read-only")

	// A write into that same shard must not build on the preloaded entry.
	require.NoError(t, snapshot.Update(func(tx *bbolt.Tx) error {
		return idx.InsertBlock(tx, recentBlock(idC, base.Add(2*time.Minute), "svc-c"))
	}))

	// Every string reference must still resolve to the right value on disk.
	fresh := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, snapshot.View(func(tx *bbolt.Tx) error {
		metas, err := fresh.GetBlocks(tx, &metastorev1.BlockList{
			Tenant: "tenant-a", Shard: 1,
			Blocks: []string{idA, idB, idC},
		})
		require.NoError(t, err)

		want := map[string]string{
			idA: "svc-a",
			idB: "svc-b",
			idC: "svc-c",
		}
		require.Len(t, metas, len(want))
		for _, m := range metas {
			require.Equal(t, want[m.Id], m.StringTable[m.Datasets[0].Labels[2]],
				"service_name of block %s", m.Id)
		}
		return nil
	}))

	// And the query path agrees with what is on disk.
	require.NoError(t, snapshot.View(func(tx *bbolt.Tx) error {
		_, err := idx.QueryMetadata(tx, context.Background(), MetadataQuery{
			Expr:      `{service_name=~".+"}`,
			StartTime: base.Add(-time.Hour),
			EndTime:   base.Add(time.Hour),
			Tenant:    []string{"tenant-a"},
			Labels:    []string{"service_name"},
		})
		return err
	}))
}
