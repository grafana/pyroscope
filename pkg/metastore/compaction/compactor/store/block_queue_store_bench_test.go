package store

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/store"
)

// BenchmarkUnmarshalBlockEntry isolates the per-entry cost of the queue scan.
// The scan is the unbounded part of the compaction page: it reads one entry
// per block awaiting compaction.
//
// Note that the entry identifier is materialised for every entry, while the
// only consumer of the listing, ListQueues, aggregates by tenant, shard, and
// level and never reads it.
func BenchmarkUnmarshalBlockEntry(b *testing.B) {
	key := make([]byte, 8+26)
	binary.BigEndian.PutUint64(key, 42)
	copy(key[8:], "01M14VWWJJEDW4TPDEFH5H5XWJ")

	value := make([]byte, 16+len("tenant-0001"))
	binary.BigEndian.PutUint64(value[0:8], 1700000000000000000)
	binary.BigEndian.PutUint32(value[8:12], 1)
	binary.BigEndian.PutUint32(value[12:16], 7)
	copy(value[16:], "tenant-0001")

	kv := store.KV{Key: key, Value: value}
	var dst compaction.BlockEntry

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.NoError(b, unmarshalBlockEntry(&dst, kv))
	}
}

// BenchmarkBlockEntryStats is the same entry read the way the queue
// aggregation reads it: without the identifier, and interning the tenant.
func BenchmarkBlockEntryStats(b *testing.B) {
	value := make([]byte, 16+len("tenant-0001"))
	binary.BigEndian.PutUint64(value[0:8], 1700000000000000000)
	binary.BigEndian.PutUint32(value[8:12], 1)
	binary.BigEndian.PutUint32(value[12:16], 7)
	copy(value[16:], "tenant-0001")

	x := &blockEntryStatsIterator{tenants: make(map[string]string)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x.cur.AppendedAt = int64(binary.BigEndian.Uint64(value[0:8]))
		x.cur.Level = binary.BigEndian.Uint32(value[8:12])
		x.cur.Shard = binary.BigEndian.Uint32(value[12:16])
		x.cur.Tenant = x.intern(value[16:])
	}
}
