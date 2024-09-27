package metastore

import (
	"context"
	"crypto/rand"
	"math"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_MetastoreState_GetProfileStats_NoData(t *testing.T) {
	m := initState(t)
	stats, err := m.getProfileStats("tenant", context.Background())
	require.NoError(t, err)
	require.Equal(t, &typesv1.GetProfileStatsResponse{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}, stats)
}

func Test_MetastoreState_GetProfileStats_MultipleShards(t *testing.T) {
	m := initState(t)
	_, _ = m.applyAddBlock(&raft.Log{}, &metastorev1.AddBlockRequest{Block: &metastorev1.BlockMeta{
		Id:       ulid.MustNew(ulid.Now(), rand.Reader).String(),
		Shard:    1,
		TenantId: "tenant1",
		MinTime:  20,
		MaxTime:  50,
	}})
	_, _ = m.applyAddBlock(&raft.Log{}, &metastorev1.AddBlockRequest{Block: &metastorev1.BlockMeta{
		Id:       ulid.MustNew(ulid.Now(), rand.Reader).String(),
		Shard:    1,
		TenantId: "tenant2",
		MinTime:  30,
		MaxTime:  60,
	}})
	_, _ = m.applyAddBlock(&raft.Log{}, &metastorev1.AddBlockRequest{Block: &metastorev1.BlockMeta{
		Id:       ulid.MustNew(ulid.Now(), rand.Reader).String(),
		Shard:    2,
		TenantId: "tenant1",
		MinTime:  10,
		MaxTime:  40,
	}})

	stats, err := m.getProfileStats("tenant1", context.Background())
	require.NoError(t, err)
	require.True(t, stats.DataIngested)
	require.True(t, stats.OldestProfileTime > math.MinInt64)
	require.True(t, stats.NewestProfileTime < math.MaxInt64)
}

func Benchmark_MetastoreState_GetProfileStats(b *testing.B) {
	m := initState(b)
	for s := 0; s < 64; s++ {
		// in a real and worst case we would have ~3700 blocks per shard per month
		// level 3 (1000 seconds): 86 blocks
		// level 2 (100 seconds):   9 blocks
		// level 1 (10 seconds):    9 blocks
		// level 0 (0.5 second):   19 blocks
		// total:                 123 blocks
		// monthly:              3690 blocks (usually less)
		for i := 0; i < 600; i++ { // level 0
			m.index.InsertBlock(&metastorev1.BlockMeta{
				Id:      ulid.MustNew(ulid.Now(), rand.Reader).String(),
				Shard:   uint32(s),
				MinTime: int64(i * 10),
				MaxTime: int64(i * 40),
				Datasets: []*metastorev1.Dataset{
					{
						TenantId: "tenant1",
					},
					{
						TenantId: "tenant2",
					},
				},
			})
		}
		for i := 0; i < 3400; i++ {
			m.index.InsertBlock(&metastorev1.BlockMeta{
				Id:       ulid.MustNew(ulid.Now(), rand.Reader).String(),
				Shard:    uint32(s),
				TenantId: "tenant1",
				MinTime:  int64(i * 10),
				MaxTime:  int64(i * 40),
			})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats, err := m.getProfileStats("tenant1", context.Background())
		require.NoError(b, err)
		require.True(b, stats.DataIngested)
	}
}
