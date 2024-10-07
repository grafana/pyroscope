package adaptive_placement

import (
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/tenant"
)

// Ruler builds placement rules based on distribution stats.
//
// Ruler is not safe for concurrent use: the caller should
// ensure synchronization.
type Ruler struct {
	limits   Limits
	datasets map[datasetKey]*datasetShards
}

func NewRuler(limits Limits) *Ruler {
	return &Ruler{
		limits:   limits,
		datasets: make(map[datasetKey]*datasetShards),
	}
}

func (r *Ruler) Load(rules *adaptive_placementpb.PlacementRules) {
	tenantLimits := make([]ShardingLimits, len(rules.Tenants))
	for i, t := range rules.Tenants {
		tenantLimits[i] = r.limits.ShardingLimits(t.TenantId)
	}
	for _, ds := range rules.Datasets {
		k := datasetKey{
			tenant:  rules.Tenants[ds.Tenant].TenantId,
			dataset: ds.Name,
		}
		limits := tenantLimits[ds.Tenant]
		dataset := &datasetShards{
			allocator:  newShardAllocator(limits),
			lastUpdate: rules.CreatedAt,
		}
		// NOTE(kolesnikovae): Only the target number of shards and
		// chosen load balancing strategy are loaded; the rest of the
		// dataset state will be built from scratch over time.
		dataset.loadBalancing = ds.LoadBalancing
		dataset.allocator.setTargetShards(ds.ShardLimit)
		r.datasets[k] = dataset
	}
}

func (r *Ruler) BuildRules(stats *adaptive_placementpb.DistributionStats) *adaptive_placementpb.PlacementRules {
	limits := r.limits.ShardingLimits(tenant.DefaultTenantID)
	defaults := &adaptive_placementpb.PlacementLimits{
		TenantShardLimit:  limits.TenantShards,
		DatasetShardLimit: limits.DefaultDatasetShards,
		LoadBalancing:     limits.LoadBalancing.proto(),
	}

	rules := adaptive_placementpb.PlacementRules{
		Defaults:  defaults,
		Tenants:   make([]*adaptive_placementpb.TenantPlacement, len(stats.Tenants)),
		Datasets:  make([]*adaptive_placementpb.DatasetPlacement, len(stats.Datasets)),
		CreatedAt: stats.CreatedAt,
	}

	tenantLimits := make([]ShardingLimits, len(stats.Tenants))
	for i, t := range stats.Tenants {
		tenantLimits[i] = r.limits.ShardingLimits(t.TenantId)
		rules.Tenants[i] = &adaptive_placementpb.TenantPlacement{
			TenantId: t.TenantId,
			Limits: &adaptive_placementpb.PlacementLimits{
				TenantShardLimit:  tenantLimits[i].TenantShards,
				DatasetShardLimit: tenantLimits[i].DefaultDatasetShards,
				LoadBalancing:     tenantLimits[i].LoadBalancing.proto(),
			},
		}
	}

	for i, datasetStats := range stats.Datasets {
		k := datasetKey{
			tenant:  rules.Tenants[datasetStats.Tenant].TenantId,
			dataset: datasetStats.Name,
		}
		dataset, ok := r.datasets[k]
		if !ok {
			dataset = &datasetShards{allocator: new(shardAllocator)}
			r.datasets[k] = dataset
		}
		limits = tenantLimits[datasetStats.Tenant]
		placement := dataset.placement(datasetStats, limits, stats.CreatedAt)
		rules.Datasets[i] = placement
	}

	return &rules
}

func (r *Ruler) Expire(before time.Time) {
	for k, ds := range r.datasets {
		if time.Unix(0, ds.lastUpdate).Before(before) {
			delete(r.datasets, k)
		}
	}
}

type datasetKey struct{ tenant, dataset string }

type datasetShards struct {
	allocator *shardAllocator
	// Load balancing strategy chosen by ruler.
	loadBalancing adaptive_placementpb.LoadBalancing
	// Last time the dataset was updated, according
	// to the stats update time.
	lastUpdate int64
}

func (ds *datasetShards) placement(
	stats *adaptive_placementpb.DatasetStats,
	limits ShardingLimits,
	now int64,
) *adaptive_placementpb.DatasetPlacement {
	ds.lastUpdate = now
	ds.allocator.setLimits(limits)
	if limits.LoadBalancing != DynamicLoadBalancing {
		ds.loadBalancing = limits.LoadBalancing.proto()
	} else if limits.LoadBalancing.needsDynamicBalancing(ds.loadBalancing) {
		ds.loadBalancing = loadBalancingStrategy(stats, ds.allocator.unitSize).proto()
	}
	return &adaptive_placementpb.DatasetPlacement{
		Tenant:        stats.Tenant,
		Name:          stats.Name,
		ShardLimit:    uint32(ds.allocator.observe(sum(stats.Usage), now)),
		LoadBalancing: ds.loadBalancing,
	}
}
