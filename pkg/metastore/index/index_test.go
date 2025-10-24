package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/pkg/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestIndex_PartitionList(t *testing.T) {
	const testTenant = "tenant"

	t.Run("new shard", func(t *testing.T) {
		db := test.BoltDB(t)
		idx := NewIndex(util.Logger, NewStore(), DefaultConfig)
		require.NoError(t, db.Update(idx.Init))

		shardID := uint32(42)
		blockMeta := &metastorev1.BlockMeta{
			Id:          test.ULID("2024-09-11T07:00:00.001Z"),
			Tenant:      1,
			Shard:       shardID,
			MinTime:     test.UnixMilli("2024-09-11T07:00:00.000Z"),
			MaxTime:     test.UnixMilli("2024-09-11T09:00:00.000Z"),
			CreatedBy:   1,
			StringTable: []string{"", testTenant, "ingester"},
		}

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return idx.InsertBlock(tx, blockMeta.CloneVT())
		}))

		p := indexstore.NewPartition(test.Time("2024-09-11T07:00:00.001Z"), idx.config.partitionDuration)
		findPartition(t, db, idx, p)
		shard := findShard(t, db, p, testTenant, shardID)
		assert.Equal(t, blockMeta.MinTime, shard.ShardIndex.MinTime)
		assert.Equal(t, blockMeta.MaxTime, shard.ShardIndex.MaxTime)
	})

	t.Run("shard update", func(t *testing.T) {
		db := test.BoltDB(t)
		idx := NewIndex(util.Logger, NewStore(), DefaultConfig)

		p := indexstore.NewPartition(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour)
		tenant := testTenant
		shardID := uint32(1)

		blockMeta := &metastorev1.BlockMeta{
			Id:          test.ULID("2024-09-11T07:00:00.001Z"),
			Tenant:      1,
			Shard:       shardID,
			MinTime:     test.UnixMilli("2024-09-11T07:00:00.000Z"),
			MaxTime:     test.UnixMilli("2024-09-11T09:00:00.000Z"),
			CreatedBy:   1,
			StringTable: []string{"", tenant, "ingester"},
		}

		require.NoError(t, db.Update(idx.Init))
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return idx.InsertBlock(tx, blockMeta.CloneVT())
		}))

		idx = NewIndex(util.Logger, NewStore(), DefaultConfig)
		require.NoError(t, db.View(idx.Restore))

		findPartition(t, db, idx, p)
		shard := findShard(t, db, p, tenant, shardID)
		assert.Equal(t, blockMeta.MinTime, shard.ShardIndex.MinTime)
		assert.Equal(t, blockMeta.MaxTime, shard.ShardIndex.MaxTime)

		newBlockMeta := &metastorev1.BlockMeta{
			Id:          test.ULID("2024-09-11T08:00:00.001Z"),
			Tenant:      1,
			Shard:       shardID,
			MinTime:     test.UnixMilli("2024-09-11T06:30:00.000Z"),
			MaxTime:     test.UnixMilli("2024-09-11T10:00:00.000Z"),
			CreatedBy:   1,
			StringTable: []string{"", tenant, "ingester"},
		}

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return idx.InsertBlock(tx, newBlockMeta.CloneVT())
		}))

		updated := findShard(t, db, p, tenant, shardID)
		assert.Equal(t, newBlockMeta.MinTime, updated.ShardIndex.MinTime)
		assert.Equal(t, newBlockMeta.MaxTime, updated.ShardIndex.MaxTime)

		require.NoError(t, db.View(func(tx *bbolt.Tx) error {
			s, err := idx.shards.getForRead(tx, p, tenant, shardID)
			if err != nil {
				return err
			}
			require.NotNil(t, s)
			assert.Equal(t, s.ShardIndex.MinTime, updated.ShardIndex.MinTime)
			assert.Equal(t, s.ShardIndex.MaxTime, updated.ShardIndex.MaxTime)
			return nil
		}))
	})
}

func findPartition(t *testing.T, db *bbolt.DB, idx *Index, k indexstore.Partition) {
	var p indexstore.Partition
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		for partition := range idx.Partitions(tx) {
			if partition.Equal(k) {
				p = partition
				break
			}
		}
		return nil
	}))
	assert.NotZero(t, p)
}

func findShard(t *testing.T, db *bbolt.DB, partition indexstore.Partition, tenant string, shardID uint32) indexstore.Shard {
	var s indexstore.Shard
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		for shard := range partition.Query(tx).Shards(tenant) {
			if shard.Shard == shardID {
				s = shard
				break
			}
		}
		return nil
	}))
	assert.NotZero(t, s)
	return s
}

func TestIndex_RestoreTimeBasedLoading(t *testing.T) {
	db := test.BoltDB(t)
	config := DefaultConfig
	config.queryLookaroundPeriod = time.Hour

	idx := NewIndex(util.Logger, NewStore(), config)
	require.NoError(t, db.Update(idx.Init))

	now := time.Now()
	const testTenant = "test-tenant"

	t1 := now.Add(-30 * time.Minute)
	t2 := now.Add(-25 * time.Hour)
	t3 := now.Add(25 * time.Hour)

	blocks := []*metastorev1.BlockMeta{
		{
			Id:          test.ULID(t1.Format(time.RFC3339)),
			Tenant:      1,
			Shard:       1,
			MinTime:     t1.UnixMilli(),
			MaxTime:     now.Add(time.Hour).UnixMilli(),
			StringTable: []string{"", testTenant},
		},

		{
			Id:          test.ULID(t2.Format(time.RFC3339)),
			Tenant:      1,
			Shard:       2,
			MinTime:     t2.UnixMilli(),
			MaxTime:     t2.Add(time.Hour).UnixMilli(),
			StringTable: []string{"", testTenant},
		},
		{
			Id:          test.ULID(t3.Format(time.RFC3339)),
			Tenant:      1,
			Shard:       3,
			MinTime:     t3.UnixMilli(),
			MaxTime:     t3.Add(time.Hour).UnixMilli(),
			StringTable: []string{"", testTenant},
		},
	}

	for _, block := range blocks {
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return idx.InsertBlock(tx, block)
		}))
	}

	idx = NewIndex(util.Logger, NewStore(), config)
	require.NoError(t, db.Update(idx.Restore))
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		s, _ := idx.shards.cache.Get(shardCacheKey{indexstore.NewPartition(t1, config.partitionDuration), testTenant, 1})
		assert.NotNil(t, s)
		s, _ = idx.shards.cache.Get(shardCacheKey{indexstore.NewPartition(t2, config.partitionDuration), testTenant, 2})
		assert.Nil(t, s)
		s, _ = idx.shards.cache.Get(shardCacheKey{indexstore.NewPartition(t3, config.partitionDuration), testTenant, 3})
		assert.Nil(t, s)
		return nil
	}))
}

func TestShardIterator_TimeFiltering(t *testing.T) {
	db := test.BoltDB(t)
	config := DefaultConfig
	config.queryLookaroundPeriod = 0
	idx := NewIndex(util.Logger, NewStore(), config)
	require.NoError(t, db.Update(idx.Init))

	tenant := "test"
	blocks := []*metastorev1.BlockMeta{
		{
			Id:          test.ULID("2024-01-01T10:00:00.000Z"),
			Tenant:      1,
			Shard:       1,
			MinTime:     test.UnixMilli("2024-01-01T10:00:00.000Z"),
			MaxTime:     test.UnixMilli("2024-01-01T11:00:00.000Z"),
			StringTable: []string{"", tenant},
		},
		{
			Id:          test.ULID("2024-01-01T12:00:00.000Z"),
			Tenant:      1,
			Shard:       2,
			MinTime:     test.UnixMilli("2024-01-01T12:00:00.000Z"),
			MaxTime:     test.UnixMilli("2024-01-01T13:00:00.000Z"),
			StringTable: []string{"", tenant},
		},
	}

	for _, block := range blocks {
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error { return idx.InsertBlock(tx, block) }))
	}

	testCases := []struct {
		name      string
		startTime string
		endTime   string
		expected  []uint32
	}{
		{"overlap first", "2024-01-01T10:30:00.000Z", "2024-01-01T10:45:00.000Z", []uint32{1}},
		{"overlap second", "2024-01-01T12:30:00.000Z", "2024-01-01T12:45:00.000Z", []uint32{2}},
		{"no overlap", "2024-01-01T15:00:00.000Z", "2024-01-01T16:00:00.000Z", []uint32{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var loaded []uint32
			require.NoError(t, db.View(func(tx *bbolt.Tx) error {
				iter := newShardIterator(tx, idx, test.Time(tc.startTime), test.Time(tc.endTime), tenant)
				for iter.Next() {
					loaded = append(loaded, iter.At().Shard)
				}
				return iter.Err()
			}))
			assert.ElementsMatch(t, tc.expected, loaded)
		})
	}
}

func TestIndex_DeleteShard(t *testing.T) {
	const baseTime = "2024-01-01T10:00:00.000Z"

	createBlock := func(tenant string, shard uint32, offset time.Duration) *metastorev1.BlockMeta {
		ts := test.Time(baseTime).Add(offset)
		return &metastorev1.BlockMeta{
			Id:          test.ULID(ts.Format(time.RFC3339)),
			Tenant:      1,
			Shard:       shard,
			MinTime:     ts.UnixMilli(),
			MaxTime:     ts.Add(time.Hour).UnixMilli(),
			StringTable: []string{"", tenant},
		}
	}

	insertBlocks := func(t *testing.T, db *bbolt.DB, idx *Index, blocks []*metastorev1.BlockMeta) {
		for _, block := range blocks {
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				return idx.InsertBlock(tx, block)
			}))
		}
	}

	assertShard := func(t *testing.T, db *bbolt.DB, p indexstore.Partition, tenant string, shard uint32, exists bool) {
		require.NoError(t, db.View(func(tx *bbolt.Tx) error {
			q := p.Query(tx)
			if q == nil && !exists {
				return nil
			}
			require.NotNil(t, q)

			var found bool
			for s := range q.Shards(tenant) {
				if s.Shard == shard {
					found = true
					break
				}
			}

			assert.Equal(t, exists, found)
			return nil
		}))
	}

	t.Run("basic deletion", func(t *testing.T) {
		db := test.BoltDB(t)
		idx := NewIndex(util.Logger, NewStore(), DefaultConfig)
		require.NoError(t, db.Update(idx.Init))

		tenant := "test-tenant"
		blocks := []*metastorev1.BlockMeta{
			createBlock(tenant, 1, 0),
			createBlock(tenant, 2, 30*time.Minute),
		}

		insertBlocks(t, db, idx, blocks)
		p := indexstore.NewPartition(test.Time(baseTime), idx.config.partitionDuration)

		assertShard(t, db, p, tenant, 1, true)
		assertShard(t, db, p, tenant, 2, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return idx.DeleteShard(tx, p, tenant, 1)
		}))

		assertShard(t, db, p, tenant, 1, false)
		assertShard(t, db, p, tenant, 2, true)

		k := shardCacheKey{partition: p, tenant: tenant, shard: 1}
		cached, found := idx.shards.cache.Get(k)
		assert.False(t, found)
		assert.Nil(t, cached)
	})

	t.Run("delete non-existent shard", func(t *testing.T) {
		db := test.BoltDB(t)
		idx := NewIndex(util.Logger, NewStore(), DefaultConfig)
		require.NoError(t, db.Update(idx.Init))

		p := indexstore.NewPartition(test.Time(baseTime), idx.config.partitionDuration)
		err := db.Update(func(tx *bbolt.Tx) error {
			return idx.DeleteShard(tx, p, "non-existent", 999)
		})
		assert.NoError(t, err)
	})

	t.Run("multiple tenants isolation", func(t *testing.T) {
		db := test.BoltDB(t)
		idx := NewIndex(util.Logger, NewStore(), DefaultConfig)
		require.NoError(t, db.Update(idx.Init))

		tenant1, tenant2 := "tenant-1", "tenant-2"
		blocks := []*metastorev1.BlockMeta{
			createBlock(tenant1, 1, 0),
			createBlock(tenant2, 1, 30*time.Minute),
		}

		insertBlocks(t, db, idx, blocks)
		p := indexstore.NewPartition(test.Time(baseTime), idx.config.partitionDuration)

		assertShard(t, db, p, tenant1, 1, true)
		assertShard(t, db, p, tenant2, 1, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return idx.DeleteShard(tx, p, tenant1, 1)
		}))

		assertShard(t, db, p, tenant1, 1, false)
		assertShard(t, db, p, tenant2, 1, true)
	})
}

func TestIndex_GetTenantStats(t *testing.T) {
	const (
		existingTenant = "tenant"
	)
	var (
		minTime = test.UnixMilli("2024-09-11T07:00:00.000Z")
		maxTime = test.UnixMilli("2024-09-11T09:00:00.000Z")
	)

	db := test.BoltDB(t)
	idx := NewIndex(util.Logger, NewStore(), DefaultConfig)
	require.NoError(t, db.Update(idx.Init))

	shardID := uint32(42)
	blockMeta := &metastorev1.BlockMeta{
		Id:          test.ULID("2024-09-11T07:00:00.001Z"),
		Tenant:      1,
		Shard:       shardID,
		MinTime:     minTime,
		MaxTime:     maxTime,
		CreatedBy:   1,
		StringTable: []string{"", existingTenant, "ingester"},
	}

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return idx.InsertBlock(tx, blockMeta.CloneVT())
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		stats := idx.GetTenantStats(tx, existingTenant)
		assert.Equal(t, true, stats.GetDataIngested())
		assert.Equal(t, minTime, stats.GetOldestProfileTime())
		assert.Equal(t, maxTime, stats.GetNewestProfileTime())
		return nil
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		stats := idx.GetTenantStats(tx, "tenant-never-sent")
		assert.Equal(t, false, stats.GetDataIngested())
		assert.Equal(t, int64(0), stats.GetOldestProfileTime())
		assert.Equal(t, int64(0), stats.GetNewestProfileTime())
		return nil
	}))

}
