package adaptive_placement

import (
	"math/rand"
	"sync"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

type Limits interface {
	DistributorTenantShards(tenant string) int
	DistributorDefaultDatasetShards(tenant string) int
	DistributorUnitSize(tenant string) int
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
	}
}

func (a *AdaptivePlacement) PlacementPolicy(k placement.Key) placement.Policy {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, ok := a.tenants[k.TenantID]
	if !ok {
		return placement.Policy{
			TenantShards:  a.limits.DistributorTenantShards(k.TenantID),
			DatasetShards: a.limits.DistributorDefaultDatasetShards(k.TenantID),
			PickShard: func(n int) int {
				return fingerprintMod(k, n)
			},
		}
	}
	d, ok := t.datasets[k.DatasetName]
	if !ok {
		return placement.Policy{
			TenantShards:  a.limits.DistributorTenantShards(k.TenantID),
			DatasetShards: a.limits.DistributorDefaultDatasetShards(k.TenantID),
			PickShard: func(n int) int {
				return fingerprintMod(k, n)
			},
		}
	}
	return placement.Policy{
		TenantShards:  a.limits.DistributorTenantShards(k.TenantID),
		DatasetShards: d.shards,
		PickShard: func(n int) int {
			return d.pick(k, n)
		},
	}
}

func (a *AdaptivePlacement) Load(p *adaptive_placementpb.Placement) {
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

func BuildPlacement(
	stats *adaptive_placementpb.DistributionStats,
	limits Limits,
) *adaptive_placementpb.Placement {
	p := adaptive_placementpb.Placement{
		Tenants:  make([]*adaptive_placementpb.TenantPlacement, 0, len(stats.Tenants)),
		Datasets: make([]*adaptive_placementpb.DatasetPlacement, 0, len(stats.Datasets)),
	}
	for _, ts := range stats.Tenants {
		p.Tenants = append(p.Tenants, &adaptive_placementpb.TenantPlacement{
			TenantId: ts.TenantId,
		})
	}
	for _, ds := range stats.Datasets {
		var sum uint64
		for _, v := range ds.Usage {
			sum += v
		}
		// TODO(kolesnikovae): once per tenant.
		unitSize := uint64(limits.DistributorUnitSize(stats.Tenants[ds.Tenant].TenantId))
		p.Datasets = append(p.Datasets, &adaptive_placementpb.DatasetPlacement{
			Tenant:      ds.Tenant,
			DatasetName: ds.Name,
			// TODO(kolesnikovae): Hysteresis.
			ShardsLimit: uint32(sum/unitSize + 1),
			// TODO(kolesnikvoae): analyze distribution over dataset shards and use round robin if needed.
			LoadBalancing: adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT_MOD,
		})
	}
	return nil
}
