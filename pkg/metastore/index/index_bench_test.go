package index

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/util"
)

func BenchmarkIndex_GetTenantStats(b *testing.B) {
	const (
		partitionDuration = 6 * time.Hour
		numPartitions     = 6 * 4 * 30
		numTenants        = 100
		numShards         = 1
	)

	db := test.BoltDB(b)
	defer func() {
		require.NoError(b, db.Close())
	}()

	config := DefaultConfig
	config.partitionDuration = partitionDuration
	config.ShardCacheSize = 1000
	config.BlockReadCacheSize = 1000
	config.BlockWriteCacheSize = 1000

	idx := NewIndex(util.Logger, NewStore(), config)
	require.NoError(b, db.Update(idx.Init))

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var blocks int

	for i := 0; i < numPartitions; i++ {
		start := startTime.Add(time.Duration(i) * partitionDuration)
		for j := 0; j < numTenants; j++ {
			tenant := fmt.Sprintf("tenant-%03d", j)
			for shard := 0; shard < numShards; shard++ {
				ts := start.Add(time.Duration(shard) * time.Hour)
				md := &metastorev1.BlockMeta{
					FormatVersion: 1,
					Id:            test.ULID(ts.Format(time.RFC3339)),
					Tenant:        1,
					Shard:         uint32(shard),
					MinTime:       ts.UnixMilli(),
					MaxTime:       ts.Add(partitionDuration / 2).UnixMilli(),
					StringTable:   []string{"", tenant},
				}
				err := db.Update(func(tx *bbolt.Tx) error {
					return idx.InsertBlock(tx, md)
				})
				require.NoError(b, err)
				blocks++
			}
		}

		if (i+1)%100 == 0 {
			b.Logf("Created %d/%d partitions (%d blocks so far)", i+1, numPartitions, blocks)
		}
	}

	for _, tc := range []struct {
		name   string
		tenant string
		desc   string
	}{
		{
			name:   "ExistingTenant",
			tenant: "tenant-000",
			desc:   "Tenant with data in all partitions",
		},
		{
			name:   "MidTenant",
			tenant: "tenant-050",
			desc:   "Tenant in the middle range",
		},
		{
			name:   "LastTenant",
			tenant: "tenant-099",
			desc:   "Last tenant in the range",
		},
		{
			name:   "NonExistentTenant",
			tenant: "tenant-999",
			desc:   "Tenant that doesn't exist",
		},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var stats *metastorev1.TenantStats
				err := db.View(func(tx *bbolt.Tx) error {
					stats = idx.GetTenantStats(tx, tc.tenant)
					return nil
				})
				require.NoError(b, err)
				if tc.tenant != "tenant-999" {
					require.True(b, stats.DataIngested)
					require.NotEqual(b, int64(0), stats.OldestProfileTime)
					require.NotEqual(b, int64(0), stats.NewestProfileTime)
				}
			}
		})
	}
}
