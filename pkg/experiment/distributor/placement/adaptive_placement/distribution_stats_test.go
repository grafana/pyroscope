package adaptive_placement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/iter"
)

func Test_StatsTracker(t *testing.T) {
	const window = time.Second * 10
	stats := NewDistributionStats(window)
	var now time.Duration

	for ; now < window; now += time.Second {
		stats.recordStats(now.Nanoseconds(), iter.NewSliceIterator([]Sample{
			{TenantID: "tenant-a", DatasetName: "dataset-a", ShardID: 1, Size: 10},
			{TenantID: "tenant-a", DatasetName: "dataset-b", ShardID: 1, Size: 10},
		}))
	}

	// Note that we deal with half-life exponent decay here.
	// Therefore, in 10s we only reached 50% of the target value.
	expected := &adaptive_placementpb.DistributionStats{
		Tenants: []*adaptive_placementpb.TenantStats{
			{TenantId: "tenant-a"},
		},
		Datasets: []*adaptive_placementpb.DatasetStats{
			{
				Tenant: 0,
				Name:   "dataset-a",
				Shards: []uint32{0},
				Usage:  []uint64{5},
			},
			{
				Tenant: 0,
				Name:   "dataset-b",
				Shards: []uint32{0},
				Usage:  []uint64{5},
			},
		},
		Shards: []*adaptive_placementpb.ShardStats{
			{Id: 1},
		},
		CreatedAt: now.Nanoseconds(),
	}
	assert.Equal(t, expected.String(), stats.build(now.Nanoseconds()).String())

	// Reassign dataset-a to shard 2 and add dataset-c.
	for ; now < time.Second*20; now += time.Second {
		stats.recordStats(now.Nanoseconds(),
			iter.NewSliceIterator([]Sample{
				{TenantID: "tenant-a", DatasetName: "dataset-a", ShardID: 2, Size: 10}, // Moved from shard 1.
				{TenantID: "tenant-a", DatasetName: "dataset-b", ShardID: 1, Size: 10}, // Not changed.
				{TenantID: "tenant-b", DatasetName: "dataset-c", ShardID: 2, Size: 10}, // Added.
			}))
	}
	expected = &adaptive_placementpb.DistributionStats{
		Tenants: []*adaptive_placementpb.TenantStats{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
		},
		Datasets: []*adaptive_placementpb.DatasetStats{
			{
				Tenant: 0,
				Name:   "dataset-a",
				Shards: []uint32{0, 1},
				Usage:  []uint64{3, 5},
			},
			{
				Tenant: 0,
				Name:   "dataset-b",
				Shards: []uint32{0},
				Usage:  []uint64{8},
			},
			{
				Tenant: 1,
				Name:   "dataset-c",
				Shards: []uint32{1},
				Usage:  []uint64{5},
			},
		},
		Shards: []*adaptive_placementpb.ShardStats{
			{Id: 1},
			{Id: 2},
		},
		CreatedAt: now.Nanoseconds(),
	}
	assert.Equal(t, expected.String(), stats.build(now.Nanoseconds()).String())

	// Next 30 seconds nothing changes.
	for ; now < time.Minute; now += time.Second {
		stats.recordStats(now.Nanoseconds(),
			iter.NewSliceIterator([]Sample{
				{TenantID: "tenant-a", DatasetName: "dataset-a", ShardID: 2, Size: 10},
				{TenantID: "tenant-a", DatasetName: "dataset-b", ShardID: 1, Size: 10},
				{TenantID: "tenant-b", DatasetName: "dataset-c", ShardID: 2, Size: 10},
			}))
	}
	expected = &adaptive_placementpb.DistributionStats{
		Tenants: []*adaptive_placementpb.TenantStats{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
		},
		Datasets: []*adaptive_placementpb.DatasetStats{
			{
				Tenant: 0,
				Name:   "dataset-a",
				Shards: []uint32{0, 1},
				Usage:  []uint64{0, 10},
			},
			{
				Tenant: 0,
				Name:   "dataset-b",
				Shards: []uint32{0},
				Usage:  []uint64{10},
			},
			{
				Tenant: 1,
				Name:   "dataset-c",
				Shards: []uint32{1},
				Usage:  []uint64{10},
			},
		},
		Shards: []*adaptive_placementpb.ShardStats{
			{Id: 1},
			{Id: 2},
		},
		CreatedAt: now.Nanoseconds(),
	}
	assert.Equal(t, expected.String(), stats.build(now.Nanoseconds()).String())

	// See what happens when a stale counter is removed (dataset-a, shard 1).
	stats.Expire(time.Unix(40, 0))
	s := stats.build(now.Nanoseconds())
	expected = &adaptive_placementpb.DistributionStats{
		Tenants: []*adaptive_placementpb.TenantStats{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
		},
		Datasets: []*adaptive_placementpb.DatasetStats{
			{
				Tenant: 0,
				Name:   "dataset-a",
				Shards: []uint32{0},
				Usage:  []uint64{10},
			},
			{
				Tenant: 0,
				Name:   "dataset-b",
				Shards: []uint32{1},
				Usage:  []uint64{10},
			},
			{
				Tenant: 1,
				Name:   "dataset-c",
				Shards: []uint32{0},
				Usage:  []uint64{10},
			},
		},
		Shards: []*adaptive_placementpb.ShardStats{
			{Id: 2},
			{Id: 1},
		},
		CreatedAt: now.Nanoseconds(),
	}
	assert.Equal(t, expected.String(), stats.build(now.Nanoseconds()).String())

	// Expire all counters.
	stats.Expire(time.Now())
	s = stats.build(now.Nanoseconds())
	assert.Empty(t, s.Tenants)
	assert.Empty(t, s.Datasets)
	assert.Empty(t, s.Shards)
	assert.Empty(t, stats.counters)
}
