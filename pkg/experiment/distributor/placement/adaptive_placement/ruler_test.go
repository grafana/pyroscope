package adaptive_placement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

type mockLimits struct{ mock.Mock }

func (m *mockLimits) PlacementLimits(tenant string) PlacementLimits {
	return m.Called(tenant).Get(0).(PlacementLimits)
}

func Test_Ruler(t *testing.T) {
	const unitSize = 512 << 10
	defaults := PlacementLimits{
		TenantShards:         10,
		DefaultDatasetShards: 2,
		MinDatasetShards:     1,
		MaxDatasetShards:     10,
		UnitSizeBytes:        unitSize,
		BurstWindow:          17 * time.Minute,
		DecayWindow:          19 * time.Minute,
		LoadBalancing:        DynamicLoadBalancing,
	}

	withDefaults := func(fn func(*PlacementLimits)) PlacementLimits {
		limits := defaults
		fn(&limits)
		return limits
	}

	defaultLimits := withDefaults(func(l *PlacementLimits) {})

	m := new(mockLimits)
	m.On("PlacementLimits", "tenant-a").
		Return(withDefaults(func(l *PlacementLimits) {
			l.TenantShards = 20
			l.DefaultDatasetShards = 3
		}))

	m.On("PlacementLimits", "tenant-b").Return(withDefaults(func(l *PlacementLimits) {
		l.LoadBalancing = FingerprintLoadBalancing
	}))

	m.On("PlacementLimits", "tenant-c").Return(defaultLimits)

	m.On("PlacementLimits", "tenant-d").Return(withDefaults(func(l *PlacementLimits) {
		l.MinDatasetShards = 5
		l.LoadBalancing = RoundRobinLoadBalancing
	}))

	oldRules := &adaptive_placementpb.PlacementRules{
		Tenants: []*adaptive_placementpb.TenantPlacement{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
			{TenantId: "tenant-c"},
		},
		Datasets: []*adaptive_placementpb.DatasetPlacement{
			{
				Tenant:            0,
				Name:              "dataset-a",
				TenantShardLimit:  2,
				DatasetShardLimit: 5,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:            1,
				Name:              "dataset-b",
				TenantShardLimit:  2,
				DatasetShardLimit: 3,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:            2,
				Name:              "dataset-c",
				TenantShardLimit:  2,
				DatasetShardLimit: 3,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
			},
		},
	}

	stats := &adaptive_placementpb.DistributionStats{
		Tenants: []*adaptive_placementpb.TenantStats{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
			{TenantId: "tenant-c"},
			{TenantId: "tenant-d"},
		},
		Datasets: []*adaptive_placementpb.DatasetStats{
			{
				Tenant: 0,
				Name:   "dataset-a",
				Shards: []uint32{0, 1, 2, 3, 4},
				Usage:  []uint64{unitSize, unitSize, unitSize, unitSize, unitSize / 2},
			},
			{
				Tenant: 1,
				Name:   "dataset-b",
				Shards: []uint32{0, 1, 2},
				Usage:  []uint64{unitSize, unitSize, unitSize},
			},
			{
				Tenant: 2,
				Name:   "dataset-c",
				Shards: []uint32{0, 1, 2},
				Usage:  []uint64{unitSize, unitSize, unitSize / 2},
			},
			{
				Tenant: 3,
				Name:   "dataset-d",
				Shards: []uint32{0},
				Usage:  []uint64{unitSize},
			},
		},
		Shards: []*adaptive_placementpb.ShardStats{
			{Id: 1, Owner: "node-a"},
			{Id: 2, Owner: "node-b"},
			{Id: 3, Owner: "node-c"},
			{Id: 4, Owner: "node-a"},
			{Id: 5, Owner: "node-c"},
		},
		CreatedAt: 1,
	}

	expected := &adaptive_placementpb.PlacementRules{
		CreatedAt: 1,
		Tenants: []*adaptive_placementpb.TenantPlacement{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
			{TenantId: "tenant-c"},
			{TenantId: "tenant-d"},
		},
		Datasets: []*adaptive_placementpb.DatasetPlacement{
			{
				Tenant:            0,
				Name:              "dataset-a",
				TenantShardLimit:  20,
				DatasetShardLimit: 5,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:            1,
				Name:              "dataset-b",
				TenantShardLimit:  10,
				DatasetShardLimit: 4,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
			},
			{
				Tenant:            2,
				Name:              "dataset-c",
				TenantShardLimit:  10,
				DatasetShardLimit: 3,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
			},
			{
				Tenant:            3,
				Name:              "dataset-d",
				TenantShardLimit:  10,
				DatasetShardLimit: 5,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
		},
	}

	ruler := NewRuler(m)
	ruler.Load(oldRules)
	assert.Equal(t, expected.String(), ruler.BuildRules(stats).String())

	// Next update only includes tenant-a dataset-a.
	// We expect that dataset-b and dataset-c will still be present.
	update := &adaptive_placementpb.DistributionStats{
		Tenants: []*adaptive_placementpb.TenantStats{
			{TenantId: "tenant-a"},
		},
		Datasets: []*adaptive_placementpb.DatasetStats{
			{
				Tenant: 0,
				Name:   "dataset-a",
				Shards: []uint32{0, 1, 2, 3, 4},
				Usage:  []uint64{unitSize, unitSize, unitSize, unitSize, unitSize / 2},
			},
		},
		Shards: []*adaptive_placementpb.ShardStats{
			{Id: 1, Owner: "node-a"},
			{Id: 2, Owner: "node-b"},
			{Id: 3, Owner: "node-c"},
			{Id: 4, Owner: "node-a"},
			{Id: 5, Owner: "node-c"},
		},
		CreatedAt: 2,
	}

	expected.CreatedAt = 2
	assert.Equal(t, expected.String(), ruler.BuildRules(update).String())

	ruler.Expire(time.Now())
	expected = &adaptive_placementpb.PlacementRules{CreatedAt: 3}
	empty := &adaptive_placementpb.DistributionStats{CreatedAt: 3}
	assert.Equal(t, expected.String(), ruler.BuildRules(empty).String())
}
