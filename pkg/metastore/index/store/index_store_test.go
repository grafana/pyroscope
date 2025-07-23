package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/test"
)

const testTenant = "test-tenant"

func TestShard_Overlaps(t *testing.T) {
	db := test.BoltDB(t)

	store := NewIndexStore()
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return store.CreateBuckets(tx)
	}))

	partitionKey := NewPartition(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour)
	shardID := uint32(1)

	blockMinTime := test.UnixMilli("2024-09-11T07:00:00.000Z")
	blockMaxTime := test.UnixMilli("2024-09-11T09:00:00.000Z")

	blockMeta := &metastorev1.BlockMeta{
		FormatVersion: 1,
		Id:            "test-block-123",
		Tenant:        1, // Index 1 in StringTable ("test-tenant")
		Shard:         shardID,
		MinTime:       blockMinTime,
		MaxTime:       blockMaxTime,
		Datasets: []*metastorev1.Dataset{
			{
				Tenant:  1, // Index 1 in StringTable ("test-tenant")
				Name:    3, // Index 3 in StringTable ("test-dataset")
				MinTime: blockMinTime,
				MaxTime: blockMaxTime,
				// Labels format: [count, name_idx, value_idx, name_idx, value_idx, ...]
				// 2 labels: service_name="service", __profile_type__="cpu"
				Labels: []int32{2, 3, 5, 4, 6},
			},
		},
		StringTable: []string{
			"",                 // Index 0
			"test-tenant",      // Index 1
			"test-dataset",     // Index 2
			"service_name",     // Index 3
			"__profile_type__", // Index 4
			"service",          // Index 5
			"cpu",              // Index 6
		},
	}

	// store a block
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return NewShard(partitionKey, testTenant, shardID).Store(tx, blockMeta)
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		shard, err := store.LoadShard(tx, partitionKey, testTenant, shardID)
		require.NoError(t, err)
		require.NotNil(t, shard)

		assert.Equal(t, blockMinTime, shard.ShardIndex.MinTime)
		assert.Equal(t, blockMaxTime, shard.ShardIndex.MaxTime)

		testCases := []struct {
			name      string
			startTime time.Time
			endTime   time.Time
			expected  bool
		}{
			{
				name:      "complete overlap - query contains block range",
				startTime: test.Time("2024-09-11T06:30:00.000Z"),
				endTime:   test.Time("2024-09-11T10:00:00.000Z"),
				expected:  true,
			},
			{
				name:      "block contains query range",
				startTime: test.Time("2024-09-11T07:30:00.000Z"),
				endTime:   test.Time("2024-09-11T08:30:00.000Z"),
				expected:  true,
			},
			{
				name:      "partial overlap - start before block, end within block",
				startTime: test.Time("2024-09-11T06:30:00.000Z"),
				endTime:   test.Time("2024-09-11T08:00:00.000Z"),
				expected:  true,
			},
			{
				name:      "partial overlap - start within block, end after block",
				startTime: test.Time("2024-09-11T08:00:00.000Z"),
				endTime:   test.Time("2024-09-11T10:00:00.000Z"),
				expected:  true,
			},
			{
				name:      "edge case - query ends exactly at block start",
				startTime: test.Time("2024-09-11T06:00:00.000Z"),
				endTime:   test.Time("2024-09-11T07:00:00.000Z"),
				expected:  true, // Inclusive boundary check
			},
			{
				name:      "edge case - query starts exactly at block end",
				startTime: test.Time("2024-09-11T09:00:00.000Z"),
				endTime:   test.Time("2024-09-11T10:00:00.000Z"),
				expected:  true, // Inclusive boundary check
			},
			{
				name:      "no overlap - query before block",
				startTime: test.Time("2024-09-11T05:00:00.000Z"),
				endTime:   test.Time("2024-09-11T06:59:58.999Z"),
				expected:  false,
			},
			{
				name:      "no overlap - query after block",
				startTime: test.Time("2024-09-11T09:00:00.001Z"),
				endTime:   test.Time("2024-09-11T11:00:00.000Z"),
				expected:  false,
			},
			{
				name:      "exact match - same start and end times",
				startTime: test.Time("2024-09-11T07:00:00.000Z"),
				endTime:   test.Time("2024-09-11T09:00:00.000Z"),
				expected:  true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := shard.ShardIndex.Overlaps(tc.startTime, tc.endTime)
				assert.Equal(t, tc.expected, result,
					"Overlaps(%v, %v) = %v, expected %v",
					tc.startTime, tc.endTime, result, tc.expected)
			})
		}

		return nil
	}))
}

func TestIndexStore_DeleteShard(t *testing.T) {
	createBlock := func(id, tenant string, shard uint32) *metastorev1.BlockMeta {
		return &metastorev1.BlockMeta{
			Id:          id,
			Tenant:      1,
			Shard:       shard,
			MinTime:     test.UnixMilli("2024-01-01T10:00:00.000Z"),
			MaxTime:     test.UnixMilli("2024-01-01T11:00:00.000Z"),
			StringTable: []string{"", tenant},
		}
	}

	storeBlock := func(t *testing.T, db *bbolt.DB, p Partition, tenant string, shard uint32, block *metastorev1.BlockMeta) {
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return NewShard(p, tenant, shard).Store(tx, block)
		}))
	}

	assertShard := func(t *testing.T, db *bbolt.DB, store *IndexStore, p Partition, tenant string, shard uint32, exists bool) {
		require.NoError(t, db.View(func(tx *bbolt.Tx) error {
			s, err := store.LoadShard(tx, p, tenant, shard)
			if exists {
				assert.NoError(t, err)
				assert.NotNil(t, s)
			} else {
				assert.Nil(t, s)
			}
			return nil
		}))
	}

	assertPartition := func(t *testing.T, db *bbolt.DB, _ *IndexStore, p Partition, exists bool) {
		require.NoError(t, db.View(func(tx *bbolt.Tx) error {
			q := p.Query(tx)
			if exists {
				assert.NotNil(t, q)
			} else {
				assert.Nil(t, q)
			}
			return nil
		}))
	}

	t.Run("basic deletion", func(t *testing.T) {
		db := test.BoltDB(t)
		store := NewIndexStore()
		require.NoError(t, db.Update(store.CreateBuckets))

		p := NewPartition(test.Time("2024-01-01T10:00:00.000Z"), 6*time.Hour)

		storeBlock(t, db, p, testTenant, 1, createBlock("block1", testTenant, 1))
		storeBlock(t, db, p, testTenant, 2, createBlock("block2", testTenant, 2))

		assertShard(t, db, store, p, testTenant, 1, true)
		assertShard(t, db, store, p, testTenant, 2, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, testTenant, 1)
		}))

		assertShard(t, db, store, p, testTenant, 1, false)
		assertShard(t, db, store, p, testTenant, 2, true)
	})

	t.Run("delete non-existent shard", func(t *testing.T) {
		db := test.BoltDB(t)
		store := NewIndexStore()
		require.NoError(t, db.Update(store.CreateBuckets))

		p := NewPartition(test.Time("2024-01-01T10:00:00.000Z"), 6*time.Hour)

		err := db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, "non-existent", 999)
		})
		assert.NoError(t, err)
	})

	t.Run("tenant bucket cleanup", func(t *testing.T) {
		db := test.BoltDB(t)
		store := NewIndexStore()
		require.NoError(t, db.Update(store.CreateBuckets))

		p := NewPartition(test.Time("2024-01-01T10:00:00.000Z"), 6*time.Hour)

		storeBlock(t, db, p, testTenant, 1, createBlock("block1", testTenant, 1))

		assertShard(t, db, store, p, testTenant, 1, true)
		assertPartition(t, db, store, p, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, testTenant, 1)
		}))

		assertShard(t, db, store, p, testTenant, 1, false)
		assertPartition(t, db, store, p, false)
	})

	t.Run("partition bucket cleanup with multiple tenants", func(t *testing.T) {
		db := test.BoltDB(t)
		store := NewIndexStore()
		require.NoError(t, db.Update(store.CreateBuckets))

		p := NewPartition(test.Time("2024-01-01T10:00:00.000Z"), 6*time.Hour)
		tenant1, tenant2 := "tenant-1", "tenant-2"

		storeBlock(t, db, p, tenant1, 1, createBlock("block1", tenant1, 1))
		storeBlock(t, db, p, tenant2, 1, createBlock("block2", tenant2, 1))

		assertShard(t, db, store, p, tenant1, 1, true)
		assertShard(t, db, store, p, tenant2, 1, true)
		assertPartition(t, db, store, p, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, tenant1, 1)
		}))

		assertShard(t, db, store, p, tenant1, 1, false)
		assertShard(t, db, store, p, tenant2, 1, true)
		assertPartition(t, db, store, p, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, tenant2, 1)
		}))

		assertShard(t, db, store, p, tenant1, 1, false)
		assertShard(t, db, store, p, tenant2, 1, false)
		assertPartition(t, db, store, p, false)
	})

	t.Run("multiple shards same tenant", func(t *testing.T) {
		db := test.BoltDB(t)
		store := NewIndexStore()
		require.NoError(t, db.Update(store.CreateBuckets))

		p := NewPartition(test.Time("2024-01-01T10:00:00.000Z"), 6*time.Hour)

		storeBlock(t, db, p, testTenant, 1, createBlock("block1", testTenant, 1))
		storeBlock(t, db, p, testTenant, 2, createBlock("block2", testTenant, 2))
		storeBlock(t, db, p, testTenant, 3, createBlock("block3", testTenant, 3))

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, testTenant, 2)
		}))

		assertShard(t, db, store, p, testTenant, 1, true)
		assertShard(t, db, store, p, testTenant, 2, false)
		assertShard(t, db, store, p, testTenant, 3, true)
		assertPartition(t, db, store, p, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, testTenant, 1)
		}))
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p, testTenant, 3)
		}))

		assertShard(t, db, store, p, testTenant, 1, false)
		assertShard(t, db, store, p, testTenant, 2, false)
		assertShard(t, db, store, p, testTenant, 3, false)
		assertPartition(t, db, store, p, false)
	})

	t.Run("multiple partitions isolation", func(t *testing.T) {
		db := test.BoltDB(t)
		store := NewIndexStore()
		require.NoError(t, db.Update(store.CreateBuckets))

		p1 := NewPartition(test.Time("2024-01-01T10:00:00.000Z"), 6*time.Hour)
		p2 := NewPartition(test.Time("2024-01-01T16:00:00.000Z"), 6*time.Hour)

		storeBlock(t, db, p1, testTenant, 1, createBlock("block1", testTenant, 1))
		storeBlock(t, db, p2, testTenant, 1, createBlock("block2", testTenant, 1))

		assertShard(t, db, store, p1, testTenant, 1, true)
		assertShard(t, db, store, p2, testTenant, 1, true)

		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			return store.DeleteShard(tx, p1, testTenant, 1)
		}))

		assertShard(t, db, store, p1, testTenant, 1, false)
		assertShard(t, db, store, p2, testTenant, 1, true)
		assertPartition(t, db, store, p1, false)
		assertPartition(t, db, store, p2, true)
	})
}
