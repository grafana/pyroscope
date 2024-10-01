package adaptive_placement

import (
	"math/rand"
	"sync"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

type Limits interface {
	ShardingLimits(tenant string) ShardingLimits
}

type ShardingLimits struct {
	TenantShards         int
	DefaultDatasetShards int
	ShardingUnitBytes    int
}

type AdaptivePlacement struct {
	mu      sync.RWMutex
	tenants map[string]*tenant
	limits  Limits
}

type tenant struct {
	datasets map[string]*dataset
}

type dataset struct {
	shards int
	pick   func(k placement.Key, n int) int
}

func NewAdaptivePlacement(limits Limits) *AdaptivePlacement {
	return &AdaptivePlacement{
		tenants: make(map[string]*tenant),
		limits:  limits,
	}
}

func (a *AdaptivePlacement) PlacementPolicy(k placement.Key) placement.Policy {
	limits := a.limits.ShardingLimits(k.TenantID)
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, ok := a.tenants[k.TenantID]
	if !ok {
		return placement.Policy{
			TenantShards:  limits.TenantShards,
			DatasetShards: limits.DefaultDatasetShards,
			PickShard: func(n int) int {
				return fingerprintMod(k, n)
			},
		}
	}
	d, ok := t.datasets[k.DatasetName]
	if !ok {
		return placement.Policy{
			TenantShards:  limits.TenantShards,
			DatasetShards: limits.DefaultDatasetShards,
			PickShard: func(n int) int {
				return fingerprintMod(k, n)
			},
		}
	}
	return placement.Policy{
		TenantShards:  limits.TenantShards,
		DatasetShards: d.shards,
		PickShard: func(n int) int {
			return d.pick(k, n)
		},
	}
}

func (a *AdaptivePlacement) Load(p *adaptive_placementpb.PlacementRules) {
	m := make(map[string]*tenant)
	tenants := make([]*tenant, len(p.Tenants))
	for i := range p.Tenants {
		t := p.Tenants[i]
		tenants[i] = &tenant{
			datasets: make(map[string]*dataset),
		}
		m[t.TenantId] = tenants[i]
	}
	for i := range p.Datasets {
		d := p.Datasets[i]
		if int(d.Tenant) >= len(tenants) {
			continue
		}
		t := tenants[d.Tenant]
		t.datasets[d.DatasetName] = &dataset{
			shards: int(d.ShardsLimit),
			pick:   buildLBFunc(d.LoadBalancing),
		}
	}
	a.mu.Lock()
	a.tenants = m
	a.mu.Unlock()
}

func BuildPlacementRules(
	limits Limits,
	stats *adaptive_placementpb.DistributionStats,
) *adaptive_placementpb.PlacementRules {
	p := adaptive_placementpb.PlacementRules{
		Tenants:  make([]*adaptive_placementpb.TenantPlacement, 0, len(stats.Tenants)),
		Datasets: make([]*adaptive_placementpb.DatasetPlacement, 0, len(stats.Datasets)),
	}
	tenantLimits := make([]ShardingLimits, 0, len(stats.Tenants))
	for _, ts := range stats.Tenants {
		tenantLimits = append(tenantLimits, limits.ShardingLimits(ts.TenantId))
		p.Tenants = append(p.Tenants, &adaptive_placementpb.TenantPlacement{
			TenantId: ts.TenantId,
		})
	}

	// TODO(kolesnikovae): Hysteresis.
	for _, ds := range stats.Datasets {
		var sum uint64
		for _, v := range ds.Usage {
			sum += v
		}
		unitSize := uint64(tenantLimits[ds.Tenant].ShardingUnitBytes)
		p.Datasets = append(p.Datasets, &adaptive_placementpb.DatasetPlacement{
			Tenant:        ds.Tenant,
			DatasetName:   ds.Name,
			ShardsLimit:   uint32(sum/unitSize + 1),
			LoadBalancing: loadBalancingFuncForDataset(ds),
		})
	}

	return nil
}

var fingerprintMod = func(k placement.Key, n int) int { return int(k.Fingerprint) % n }

var roundRobin = func(_ placement.Key, n int) int { return rand.Intn(n) }

func buildLBFunc(lb adaptive_placementpb.LoadBalancing) func(k placement.Key, n int) int {
	switch lb {
	default:
		return fingerprintMod
	case adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN:
		return roundRobin
	}
}

func loadBalancingFuncForDataset(*adaptive_placementpb.DatasetStats) adaptive_placementpb.LoadBalancing {
	// TODO(kolesnikovae): Adaptive LoadBalancing.
	return adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT_MOD
}
