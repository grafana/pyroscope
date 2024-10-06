package adaptive_placement

import (
	"sync"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

// AdaptivePlacement is a placement policy that
// adapts to the distribution of data.
//
// It uses a set of rules to determine the number
// of shards to allocate to each tenant and dataset,
// and a load balancing function to distribute the
// dataset keys.
type AdaptivePlacement struct {
	mu       sync.RWMutex
	tenants  map[string]*tenantPlacement
	defaults *adaptive_placementpb.PlacementLimits
}

type tenantPlacement struct {
	datasets map[string]*datasetPlacement
	limits   *adaptive_placementpb.PlacementLimits
}

type datasetPlacement struct {
	shards        int
	loadBalancing LoadBalancing
}

func NewAdaptivePlacement() *AdaptivePlacement {
	return &AdaptivePlacement{
		tenants: make(map[string]*tenantPlacement),
		defaults: &adaptive_placementpb.PlacementLimits{
			TenantShardLimit:  defaultTenantShardLimit,
			DatasetShardLimit: defaultDatasetShardLimit,
			LoadBalancing:     defaultLoadBalancing.proto(),
		},
	}
}

func (a *AdaptivePlacement) PlacementPolicy(k placement.Key) placement.Policy {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, ok := a.tenants[k.TenantID]
	if !ok {
		return placement.Policy{
			TenantShards:  int(a.defaults.TenantShardLimit),
			DatasetShards: int(a.defaults.DatasetShardLimit),
			PickShard:     loadBalancingFromProto(a.defaults.LoadBalancing).pick(k),
		}
	}
	d, ok := t.datasets[k.DatasetName]
	if !ok {
		return placement.Policy{
			TenantShards:  int(t.limits.TenantShardLimit),
			DatasetShards: int(t.limits.DatasetShardLimit),
			PickShard:     loadBalancingFromProto(a.defaults.LoadBalancing).pick(k),
		}
	}
	return placement.Policy{
		TenantShards:  int(t.limits.TenantShardLimit),
		DatasetShards: d.shards,
		PickShard:     d.loadBalancing.pick(k),
	}
}

func (a *AdaptivePlacement) Load(p *adaptive_placementpb.PlacementRules) {
	m := make(map[string]*tenantPlacement)
	tenants := make([]*tenantPlacement, len(p.Tenants))
	for i := range p.Tenants {
		t := p.Tenants[i]
		limits := t.Limits
		if limits == nil {
			limits = a.defaults
		}
		tenants[i] = &tenantPlacement{
			datasets: make(map[string]*datasetPlacement),
			limits:   limits,
		}
		m[t.TenantId] = tenants[i]
	}
	for i := range p.Datasets {
		d := p.Datasets[i]
		if int(d.Tenant) >= len(tenants) {
			continue
		}
		t := tenants[d.Tenant]
		t.datasets[d.Name] = &datasetPlacement{
			shards:        int(d.ShardLimit),
			loadBalancing: loadBalancingFromProto(d.LoadBalancing),
		}
	}
	a.mu.Lock()
	a.tenants = m
	if defaults := p.GetDefaults(); defaults != nil {
		a.defaults = defaults
	}
	a.mu.Unlock()
}
