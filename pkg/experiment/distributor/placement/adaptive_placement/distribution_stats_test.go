package adaptive_placement

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

func Test_StatsTracker(t *testing.T) {
	const (
		retention = time.Minute
		window    = time.Second * 10
	)

	tracker := NewStatsTracker(window, retention)
	// A stub ID for the block is used to bypass
	// block staleness check.
	ulidNow, err := ulid.New(ulid.Now(), rand.Reader)
	require.NoError(t, err)
	stubID := ulidNow.String()

	var now time.Duration
	for ; now < window; now += time.Second {
		md := &metastorev1.BlockMeta{
			Id:    stubID,
			Shard: 1,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-a", Size: 10},
				{TenantId: "tenant-a", Name: "dataset-b", Size: 10},
			},
		}
		tracker.RecordStats(md, now.Nanoseconds())
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
	}
	assert.Equal(t, expected.String(), tracker.UpdateStats(now.Nanoseconds()).String())

	// Reassign dataset-a to shard 2 and add dataset-c.
	for ; now < time.Second*20; now += time.Second {
		tracker.RecordStats(&metastorev1.BlockMeta{
			Id:    stubID,
			Shard: 1,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-b", Size: 10}, // Not changed.
			},
		}, now.Nanoseconds())
		tracker.RecordStats(&metastorev1.BlockMeta{
			Id:    stubID,
			Shard: 2,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-a", Size: 10}, // Moved from shard 1.
				{TenantId: "tenant-b", Name: "dataset-c", Size: 10}, // Added.
			},
		}, now.Nanoseconds())
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
	}
	assert.Equal(t, expected.String(), tracker.UpdateStats(now.Nanoseconds()).String())

	// Next 30 seconds nothing changes.
	for ; now < time.Minute; now += time.Second {
		tracker.RecordStats(&metastorev1.BlockMeta{
			Id:    stubID,
			Shard: 1,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-b", Size: 10},
			},
		}, now.Nanoseconds())
		tracker.RecordStats(&metastorev1.BlockMeta{
			Id:    stubID,
			Shard: 2,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-a", Size: 10},
				{TenantId: "tenant-b", Name: "dataset-c", Size: 10},
			},
		}, now.Nanoseconds())
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
	}
	assert.Equal(t, expected.String(), tracker.UpdateStats(now.Nanoseconds()).String())

	// See what happens when a stale counter is removed (dataset-a, shard 1).
	stats := tracker.UpdateStats(retention.Nanoseconds() + 10*time.Second.Nanoseconds())
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
	}
	assert.Equal(t, expected.String(), tracker.UpdateStats(now.Nanoseconds()).String())

	// Expire all counters.
	stats = tracker.UpdateStats(2 * retention.Nanoseconds())
	assert.Empty(t, stats.Tenants)
	assert.Empty(t, stats.Datasets)
	assert.Empty(t, stats.Shards)
	assert.Empty(t, tracker.counters)
}

func Test_StatsTracker_stale_block(t *testing.T) {
	const (
		retention = time.Minute
		window    = time.Second * 10
	)

	t.Run("stale blocks are ignored", func(t *testing.T) {
		tracker := NewStatsTracker(window, retention)
		now := time.Now()
		timestamp := uint64(now.Add(-5 * time.Minute).UnixMilli())

		md := &metastorev1.BlockMeta{
			Id:    ulid.MustNew(timestamp, rand.Reader).String(),
			Shard: 1,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-b", Size: 10},
			},
		}
		tracker.RecordStats(md, now.UnixNano())
		assert.Empty(t, tracker.counters)
	})

	t.Run("invalid blocks are ignored", func(t *testing.T) {
		tracker := NewStatsTracker(window, retention)
		md := &metastorev1.BlockMeta{
			Shard: 1,
			Datasets: []*metastorev1.Dataset{
				{TenantId: "tenant-a", Name: "dataset-b", Size: 10},
			},
		}
		tracker.RecordStats(md, 0)
		assert.Empty(t, tracker.counters)
	})
}
