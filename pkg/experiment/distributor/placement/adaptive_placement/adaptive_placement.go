package adaptive_placement

import (
	"math/rand"
	"sync"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

type AdaptivePlacement struct {
	mu      sync.Mutex
	tenants map[string]*tenant

	defaultTenantShards int
}

type tenant struct {
	shards   int
	datasets map[string]*dataset

	defaultDatasetShards int
	defaultLoadBalancing func(k placement.Key, n int) int
}

type dataset struct {
	shards int
	pick   func(k placement.Key, n int) int
}

func NewAdaptivePlacement() *AdaptivePlacement {
	return &AdaptivePlacement{
		tenants: make(map[string]*tenant),

		defaultTenantShards: 3,
	}
}

func (a *AdaptivePlacement) Place(placement.Key) *placement.Placement { return nil }

func (a *AdaptivePlacement) NumTenantShards(k placement.Key, _ int) (size int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.tenants[k.TenantID]; ok {
		return t.shards
	}
	return a.defaultTenantShards
}

func (a *AdaptivePlacement) NumDatasetShards(k placement.Key, n int) (size int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.tenants[k.TenantID]; ok {
		if d, ok := t.datasets[k.DatasetName]; ok {
			return d.shards
		}
		return t.defaultDatasetShards
	}
	return a.defaultTenantShards
}

func (a *AdaptivePlacement) PickShard(k placement.Key, n int) (shard int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.tenants[k.TenantID]; ok {
		if d, ok := t.datasets[k.DatasetName]; ok {
			return d.pick(k, n)
		}
		return t.defaultLoadBalancing(k, n)
	}
	return fingerprintMod(k, n)
}

func (a *AdaptivePlacement) Load(p *adaptive_placementpb.Placement) {
	m := make(map[string]*tenant)
	tenants := make([]*tenant, len(p.Tenants))
	for i := range p.Tenants {
		t := p.Tenants[i]
		tenants[i] = &tenant{
			shards:               int(t.ShardsLimit),
			datasets:             make(map[string]*dataset),
			defaultLoadBalancing: fingerprintMod,
			defaultDatasetShards: 3,
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

func BuildPlacement(stats *adaptive_placementpb.DistributionStats) *adaptive_placementpb.Placement {
	// TODO
	return nil
}
