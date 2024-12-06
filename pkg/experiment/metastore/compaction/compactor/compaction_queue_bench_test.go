package compactor

import (
	"strconv"
	"testing"
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

func BenchmarkCompactionQueue_Push(b *testing.B) {
	s := Strategy{
		MaxBlocksPerLevel: []uint{20, 10, 10},
		MaxBlocksDefault:  defaultBlockBatchSize,
		MaxBatchAge:       defaultMaxBlockBatchAge,
	}

	q := newCompactionQueue(s, nil)
	const (
		tenants = 1
		levels  = 1
		shards  = 64
	)

	keys := make([]compactionKey, levels*tenants*shards)
	for i := range keys {
		keys[i] = compactionKey{
			tenant: strconv.Itoa(i % tenants),
			shard:  uint32(i % shards),
			level:  uint32(i % levels),
		}
	}

	writes := make([]int64, len(keys))
	now := time.Now().UnixNano()
	for i := range writes {
		writes[i] = now
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for j, key := range keys {
			q.push(compaction.BlockEntry{
				Index:      uint64(j),
				AppendedAt: writes[j],
				ID:         strconv.Itoa(j),
				Tenant:     key.tenant,
				Shard:      key.shard,
				Level:      key.level,
			})
		}
		for j := range writes {
			writes[j] += int64(time.Millisecond * 500)
		}
	}
}
