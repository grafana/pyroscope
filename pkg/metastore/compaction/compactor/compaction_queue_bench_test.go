package compactor

import (
	"strconv"
	"testing"

	"github.com/grafana/pyroscope/pkg/metastore/compaction"
)

func BenchmarkCompactionQueue_Push(b *testing.B) {
	const (
		tenants = 1
		levels  = 1
		shards  = 64
	)

	q := newCompactionQueue(DefaultConfig(), nil)
	keys := make([]compactionKey, levels*tenants*shards)
	for i := range keys {
		keys[i] = compactionKey{
			tenant: strconv.Itoa(i % tenants),
			shard:  uint32(i % shards),
			level:  uint32(i % levels),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		q.push(compaction.BlockEntry{
			Index:      uint64(i),
			AppendedAt: int64(i),
			ID:         strconv.Itoa(i),
			Tenant:     k.tenant,
			Shard:      k.shard,
			Level:      k.level,
		})
	}
}
