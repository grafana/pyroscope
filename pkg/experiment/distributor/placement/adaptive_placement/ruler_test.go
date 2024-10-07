package adaptive_placement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type mockLimits struct{ mock.Mock }

func (m *mockLimits) ShardingLimits(tenant string) ShardingLimits {
	return m.Called(tenant).Get(0).(ShardingLimits)
}

func Test_Ruler(t *testing.T) {
	const unitSize = 512 << 10
	defaults := ShardingLimits{
		TenantShards:         10,
		DefaultDatasetShards: 2,
		MinDatasetShards:     1,
		MaxDatasetShards:     10,
		UnitSizeBytes:        unitSize,
		BurstWindow:          17 * time.Minute,
		DecayWindow:          19 * time.Minute,
		LoadBalancing:        DynamicLoadBalancing,
	}

	withDefaults := func(fn func(*ShardingLimits)) ShardingLimits {
		limits := defaults
		fn(&limits)
		return limits
	}

	defaultLimits := withDefaults(func(l *ShardingLimits) {})

	m := new(mockLimits)
	m.On("ShardingLimits", tenant.DefaultTenantID).Return(defaultLimits)
	m.On("ShardingLimits", "tenant-a").
		Return(withDefaults(func(l *ShardingLimits) {
			l.TenantShards = 20
			l.DefaultDatasetShards = 3
		}))

	m.On("ShardingLimits", "tenant-b").Return(withDefaults(func(l *ShardingLimits) {
		l.LoadBalancing = FingerprintLoadBalancing
	}))

	m.On("ShardingLimits", "tenant-c").Return(defaultLimits)

	m.On("ShardingLimits", "tenant-d").Return(withDefaults(func(l *ShardingLimits) {
		l.MinDatasetShards = 5
		l.LoadBalancing = RoundRobinLoadBalancing
	}))

	old := &adaptive_placementpb.PlacementRules{
		Defaults: &adaptive_placementpb.PlacementLimits{
			TenantShardLimit:  1,
			DatasetShardLimit: 2,
		},
		Tenants: []*adaptive_placementpb.TenantPlacement{
			{
				TenantId: "tenant-a",
				Limits: &adaptive_placementpb.PlacementLimits{
					TenantShardLimit:  2,
					DatasetShardLimit: 3,
				},
			},
			{
				TenantId: "tenant-b",
				Limits:   &adaptive_placementpb.PlacementLimits{},
			},
			{
				TenantId: "tenant-c",
			},
		},
		Datasets: []*adaptive_placementpb.DatasetPlacement{
			{
				Tenant:        0,
				Name:          "dataset-a",
				ShardLimit:    5,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:        1,
				Name:          "dataset-b",
				ShardLimit:    3,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:        2,
				Name:          "dataset-c",
				ShardLimit:    3,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
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
	}

	expected := &adaptive_placementpb.PlacementRules{
		Defaults: &adaptive_placementpb.PlacementLimits{
			TenantShardLimit:  10,
			DatasetShardLimit: 2,
		},
		Tenants: []*adaptive_placementpb.TenantPlacement{
			{
				TenantId: "tenant-a",
				Limits: &adaptive_placementpb.PlacementLimits{
					TenantShardLimit:  20,
					DatasetShardLimit: 3,
				},
			},
			{
				TenantId: "tenant-b",
				Limits: &adaptive_placementpb.PlacementLimits{
					TenantShardLimit:  10,
					DatasetShardLimit: 2,
					LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
				},
			},
			{
				TenantId: "tenant-c",
				Limits: &adaptive_placementpb.PlacementLimits{
					TenantShardLimit:  10,
					DatasetShardLimit: 2,
				},
			},
			{
				TenantId: "tenant-d",
				Limits: &adaptive_placementpb.PlacementLimits{
					TenantShardLimit:  10,
					DatasetShardLimit: 2,
					LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
				},
			},
		},
		Datasets: []*adaptive_placementpb.DatasetPlacement{
			{
				Tenant:        0,
				Name:          "dataset-a",
				ShardLimit:    5,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:        1,
				Name:          "dataset-b",
				ShardLimit:    4,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
			},
			{
				Tenant:        2,
				Name:          "dataset-c",
				ShardLimit:    3,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
			},
			{
				Tenant:        3,
				Name:          "dataset-d",
				ShardLimit:    5,
				LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
		},
	}

	r := NewRuler(m)
	r.Load(old)

	assert.Equal(t, expected.String(), r.BuildRules(stats).String())
}
