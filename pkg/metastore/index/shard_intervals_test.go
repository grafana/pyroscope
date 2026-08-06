package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	indexstore "github.com/grafana/pyroscope/v2/pkg/metastore/index/store"
)

func TestShardIntervalIndex(t *testing.T) {
	index := newShardIntervalIndex()
	partition := func(hour int) indexstore.Partition {
		return indexstore.NewPartition(time.Date(2024, 1, 1, hour, 0, 0, 0, time.UTC), time.Hour)
	}
	shard := func(tenant string, p indexstore.Partition, id uint32, min, max int64) indexstore.Shard {
		return indexstore.Shard{
			Partition:  p,
			Tenant:     tenant,
			Shard:      id,
			ShardIndex: indexstore.ShardIndex{MinTime: min, MaxTime: max},
		}
	}

	first := shard("tenant-a", partition(1), 1, 10, 20)
	first.StringTable = metadata.NewStringTable()
	sameShardOtherPartition := shard("tenant-a", partition(4), 1, 10, 20)
	nested := shard("tenant-a", partition(2), 2, 5, 30)
	legacy := shard("tenant-a", partition(3), 3, 0, 0)
	otherTenant := shard("tenant-b", partition(1), 1, 10, 20)
	for _, s := range []indexstore.Shard{first, sameShardOtherPartition, nested, legacy, otherTenant} {
		index.upsert(s)
	}
	assert.Nil(t, index.tenants[first.Tenant].entries[shardIntervalKey{
		partition: first.Partition,
		tenant:    first.Tenant,
		shard:     first.Shard,
	}].shard.StringTable)
	assert.Len(t, index.tenants[first.Tenant].legacy, 1)

	candidates, ok, err := index.candidates(nil, nil, 15, 15, "tenant-a")
	require.NoError(t, err)
	require.True(t, ok)
	assert.ElementsMatch(t, []uint32{1, 1, 2, 3}, shardIDs(candidates))

	candidates, ok, err = index.candidates(nil, nil, 15, 15, "tenant-b")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []uint32{1}, shardIDs(candidates))

	// Updating an existing shard widens its persisted summary without creating
	// a duplicate tree node.
	widened := first
	widened.ShardIndex.MinTime = 1
	widened.ShardIndex.MaxTime = 40
	index.upsert(widened)
	candidates, ok, err = index.candidates(nil, nil, 35, 35, "tenant-a")
	require.NoError(t, err)
	require.True(t, ok)
	assert.ElementsMatch(t, []uint32{1, 3}, shardIDs(candidates))
}

func shardIDs(shards []indexstore.Shard) []uint32 {
	ids := make([]uint32, len(shards))
	for n, shard := range shards {
		ids[n] = shard.Shard
	}
	return ids
}
