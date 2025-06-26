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

func TestShard_Overlaps(t *testing.T) {
	db := test.BoltDB(t)

	store := NewIndexStore()
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return store.CreateBuckets(tx)
	}))

	partitionKey := NewPartition(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour)
	tenant := "test-tenant"
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
		return NewShard(partitionKey, tenant, shardID).Store(tx, blockMeta)
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		shard, err := store.LoadShard(tx, partitionKey, tenant, shardID)
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
