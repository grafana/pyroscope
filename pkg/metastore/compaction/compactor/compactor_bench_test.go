package compactor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor/store"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

// blockID builds a block identifier of the same shape and length as the ULIDs
// the queue holds, so that the scan allocates what it would in production.
func blockID(i int) string {
	return fmt.Sprintf("01M14VWWJJEDW4TPDEFH%06d", i)
}

// benchmarkQueue fills the block queue with n entries. Level 0 entries carry
// no tenant: they are the multi-tenant segments, and they dominate a backlog,
// as every block enters compaction at level 0.
func benchmarkQueue(tb testing.TB, n int) (*bbolt.DB, *Compactor) {
	db := test.BoltDB(tb)
	s := store.NewBlockQueueStore()
	c := NewCompactor(DefaultConfig(), s, nil, nil)
	require.NoError(tb, db.Update(func(tx *bbolt.Tx) error {
		if err := s.CreateBuckets(tx); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			e := compaction.BlockEntry{
				Index:      uint64(i),
				AppendedAt: int64(i) * 1000,
				ID:         blockID(i),
				Shard:      uint32(i % 64),
			}
			// One in five entries has been compacted to level 1 already and
			// belongs to one of a thousand tenants.
			if i%5 == 0 {
				e.Level = 1
				e.Tenant = fmt.Sprintf("tenant-%04d", i%1000)
			}
			if err := s.StoreEntry(tx, e); err != nil {
				return err
			}
		}
		return nil
	}))
	return db, c
}

// BenchmarkCompactor_ListQueues measures the full queue scan that every
// compaction page load performs. The entry count is not bounded by any
// configuration: it grows with the compaction backlog.
func BenchmarkCompactor_ListQueues(b *testing.B) {
	for _, n := range []int{10_000, 100_000, 1_000_000} {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			db, c := benchmarkQueue(b, n)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				require.NoError(b, db.View(func(tx *bbolt.Tx) error {
					queues, err := c.ListQueues(ctx, tx, QueueFilter{})
					require.NoError(b, err)
					require.NotEmpty(b, queues)
					return nil
				}))
			}
		})
	}
}

// BenchmarkCompactor_ListQueues_filtered measures the same scan with a tenant
// filter. The filter cannot reduce the number of entries read: it only limits
// what is aggregated and returned.
func BenchmarkCompactor_ListQueues_filtered(b *testing.B) {
	const n = 1_000_000
	db, c := benchmarkQueue(b, n)
	ctx := context.Background()
	tenant := "tenant-0005"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.NoError(b, db.View(func(tx *bbolt.Tx) error {
			_, err := c.ListQueues(ctx, tx, QueueFilter{Tenant: &tenant})
			require.NoError(b, err)
			return nil
		}))
	}
}
