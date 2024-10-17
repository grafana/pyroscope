package adaptive_placement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

func Test_AdaptivePlacement(t *testing.T) {
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

	m := new(mockLimits)
	m.On("PlacementLimits", "tenant-a").Return(withDefaults(func(l *PlacementLimits) {}))
	m.On("PlacementLimits", "tenant-b").Return(withDefaults(func(l *PlacementLimits) {
		l.TenantShards = 20
		l.DefaultDatasetShards = 3
	}))

	p := NewAdaptivePlacement(m)

	policy := p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a",
	})
	assert.Equal(t, 10, policy.TenantShards)
	assert.Equal(t, 2, policy.DatasetShards)
	assert.False(t, isRoundRobin(policy.PickShard))

	policy = p.Policy(placement.Key{
		TenantID:    "tenant-b",
		DatasetName: "dataset-b-1",
	})
	assert.Equal(t, 20, policy.TenantShards)
	assert.Equal(t, 3, policy.DatasetShards)
	assert.False(t, isRoundRobin(policy.PickShard))

	// Load new rules and override limits for tenant-a dataset-a
	p.Update(&adaptive_placementpb.PlacementRules{
		Tenants: []*adaptive_placementpb.TenantPlacement{
			{TenantId: "tenant-a"},
			{TenantId: "tenant-b"},
			{TenantId: "tenant-c"},
		},
		Datasets: []*adaptive_placementpb.DatasetPlacement{
			{
				// A placement rule may have a newer/different limit for the tenant.
				Tenant:            0,
				Name:              "dataset-a",
				TenantShardLimit:  4,
				DatasetShardLimit: 4,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN,
			},
			{
				Tenant:            0,
				Name:              "dataset-a-2",
				TenantShardLimit:  4,
				DatasetShardLimit: 1,
				LoadBalancing:     adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT,
			},
		},
		CreatedAt: 1,
	})

	// Assert that the new rules impacted the placement policy for the dataset.
	policy = p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a",
	})
	assert.Equal(t, 4, policy.TenantShards)
	assert.Equal(t, 4, policy.DatasetShards)
	assert.True(t, isRoundRobin(policy.PickShard))

	// Other datasets of the tenant are not affected.
	policy = p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-b",
	})
	assert.Equal(t, 10, policy.TenantShards)
	assert.Equal(t, 2, policy.DatasetShards)
	assert.False(t, isRoundRobin(policy.PickShard))

	policy = p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a-2",
	})
	assert.Equal(t, 4, policy.TenantShards)
	assert.Equal(t, 1, policy.DatasetShards)
	assert.False(t, isRoundRobin(policy.PickShard))

	// Other tenants are not affected.
	policy = p.Policy(placement.Key{
		TenantID:    "tenant-b",
		DatasetName: "dataset-b-1",
	})
	assert.Equal(t, 20, policy.TenantShards)
	assert.Equal(t, 3, policy.DatasetShards)
	assert.False(t, isRoundRobin(policy.PickShard))
}

// This does not test the actual round-robin behavior,
// but rather that the function is not deterministic.
func isRoundRobin(fn func(int) int) bool {
	const N = 10
	r := fn(N)
	for i := 0; i < N; i++ {
		if r != fn(N) {
			return true
		}
	}
	return false
}
