package adaptive_placement

import (
	"slices"
	"strings"
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
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
		if ds.Limits == nil {
			continue
		}
		k := datasetKey{
			tenant:  rules.Tenants[ds.Tenant].TenantId,
			dataset: ds.Name,
		}
		limits := tenantLimits[ds.Tenant]
		dataset := &datasetShards{
			allocator:  newShardAllocator(limits),
			lastUpdate: rules.CreatedAt,
			limits:     ds.Limits,
		}
		// NOTE(kolesnikovae): We prohibit decreasing the number
		// of shards for the dataset till the expiration of the
		// decay window since the moment rules were created. Thus,
		// if statistics are not available or populated slowly,
		// we won't shrink the dataset prematurely but will be
		// able to scale out if needed.
		dataset.allocator.decayOffset = rules.CreatedAt
		dataset.allocator.currentMin = int(ds.Limits.DatasetShardLimit)
		r.datasets[k] = dataset
	}
}

func (r *Ruler) BuildRules(stats *adaptive_placementpb.DistributionStats) *adaptive_placementpb.PlacementRules {
	rules := adaptive_placementpb.PlacementRules{
		Tenants:   make([]*adaptive_placementpb.TenantPlacement, len(stats.Tenants)),
		Datasets:  make([]*adaptive_placementpb.DatasetPlacement, len(stats.Datasets)),
		CreatedAt: stats.CreatedAt,
	}

	tenantLimits := make([]ShardingLimits, len(stats.Tenants))
	tenants := make(map[string]int)
	for i, t := range stats.Tenants {
		tenants[t.TenantId] = i
		tenantLimits[i] = r.limits.ShardingLimits(t.TenantId)
		rules.Tenants[i] = &adaptive_placementpb.TenantPlacement{
			TenantId: t.TenantId,
		}
	}

	for i, datasetStats := range stats.Datasets {
		k := datasetKey{
			tenant:  rules.Tenants[datasetStats.Tenant].TenantId,
			dataset: datasetStats.Name,
		}
		limits := tenantLimits[datasetStats.Tenant]
		dataset, ok := r.datasets[k]
		if !ok {
			dataset = &datasetShards{
				allocator: new(shardAllocator),
				limits: &adaptive_placementpb.PlacementLimits{
					TenantShardLimit:  limits.TenantShards,
					DatasetShardLimit: limits.DefaultDatasetShards,
					LoadBalancing:     limits.LoadBalancing.proto(),
				},
			}
			r.datasets[k] = dataset
		}
		rules.Datasets[i] = dataset.placement(datasetStats, limits, stats.CreatedAt)
	}

	// Include datasets that were not present in the current stats.
	// Although, not strictly required, we iterate over the keys
	// in a deterministic order to make the output deterministic.
	keys := make([]datasetKey, 0, len(r.datasets))
	for k, dataset := range r.datasets {
		if dataset.lastUpdate < stats.CreatedAt {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b datasetKey) int {
		return a.compare(b)
	})

	for _, k := range keys {
		dataset := r.datasets[k]
		t, ok := tenants[k.tenant]
		if !ok {
			t = len(rules.Tenants)
			tenants[k.tenant] = t
			rules.Tenants = append(rules.Tenants, &adaptive_placementpb.TenantPlacement{
				TenantId: k.tenant,
			})
		}
		rules.Datasets = append(rules.Datasets, &adaptive_placementpb.DatasetPlacement{
			Tenant: uint32(t),
			Name:   k.dataset,
			Limits: dataset.limits,
		})
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

func (k datasetKey) compare(x datasetKey) int {
	if c := strings.Compare(k.tenant, x.tenant); c != 0 {
		return c
	}
	return strings.Compare(k.dataset, x.dataset)
}

type datasetShards struct {
	limits    *adaptive_placementpb.PlacementLimits
	allocator *shardAllocator
	// Last time the dataset was updated,
	// according to the stats update time.
	lastUpdate int64
}

func (d *datasetShards) placement(
	stats *adaptive_placementpb.DatasetStats,
	limits ShardingLimits,
	now int64,
) *adaptive_placementpb.DatasetPlacement {
	d.lastUpdate = now
	d.allocator.setLimits(limits)
	d.limits.TenantShardLimit = limits.TenantShards
	d.limits.DatasetShardLimit = uint64(d.allocator.observe(sum(stats.Usage), now))
	// Determine whether we need to change the load balancing strategy.
	configured := limits
	if configured.LoadBalancing != DynamicLoadBalancing {
		d.limits.LoadBalancing = configured.LoadBalancing.proto()
	} else if configured.LoadBalancing.needsDynamicBalancing(d.limits.LoadBalancing) {
		d.limits.LoadBalancing = loadBalancingStrategy(stats, d.allocator.unitSize, d.allocator.target).proto()
	}
	return &adaptive_placementpb.DatasetPlacement{
		Tenant: stats.Tenant,
		Name:   stats.Name,
		Limits: d.limits,
	}
}
