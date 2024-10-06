package adaptive_placement

import (
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type Ruler struct {
	limits   Limits
	stats    *DistributionStats
	datasets map[datasetKey]*datasetShards
}

func NewRuler(limits Limits, window, retention time.Duration) *Ruler {
	return &Ruler{
		limits:   limits,
		stats:    NewDistributionStats(window, retention),
		datasets: make(map[datasetKey]*datasetShards),
	}
}

func (r *Ruler) RecordStats(md *metastorev1.BlockMeta) {
	r.stats.RecordStats(md, time.Now().UnixNano())
}

func (r *Ruler) BuildRules() *adaptive_placementpb.PlacementRules {
	return r.buildRules(time.Now().UnixNano())
}

func (r *Ruler) recordStats(md *metastorev1.BlockMeta, now int64) {
	r.stats.RecordStats(md, now)
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
			defaultLoadBalancing: limits.LoadBalancing,
			allocator:            newShardAllocator(limits),
		}
		// NOTE(kolesnikovae): Only the target number of shards and
		// chosen load balancing strategy are loaded; the rest of the
		// dataset state will be built from scratch over time.
		dataset.loadBalancing = ds.LoadBalancing
		dataset.allocator.target = ds.ShardLimit
		dataset.allocator.currentMin = ds.ShardLimit
		r.datasets[k] = dataset
	}
}

func (r *Ruler) buildRules(now int64) *adaptive_placementpb.PlacementRules {
	limits := r.limits.ShardingLimits(tenant.DefaultTenantID)
	defaults := &adaptive_placementpb.PlacementLimits{
		TenantShardLimit:  limits.TenantShards,
		DatasetShardLimit: limits.DefaultDatasetShards,
		LoadBalancing:     limits.LoadBalancing.proto(),
	}

	stats := r.stats.Build(now)
	rules := adaptive_placementpb.PlacementRules{
		Defaults: defaults,
		Tenants:  make([]*adaptive_placementpb.TenantPlacement, len(stats.Tenants)),
		Datasets: make([]*adaptive_placementpb.DatasetPlacement, len(stats.Datasets)),
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
			limits = tenantLimits[datasetStats.Tenant]
			dataset = &datasetShards{
				defaultLoadBalancing: limits.LoadBalancing,
				allocator:            newShardAllocator(limits),
			}
			r.datasets[k] = dataset
		}
		rules.Datasets[i] = dataset.placement(datasetStats, now)
	}

	return &rules
}

type datasetKey struct{ tenant, dataset string }

type datasetShards struct {
	allocator *shardAllocator
	// Load balancing strategy as per the tenant limits.
	defaultLoadBalancing LoadBalancing
	// Load balancing strategy chosen by ruler.
	loadBalancing adaptive_placementpb.LoadBalancing
}

func (ds *datasetShards) placement(
	stats *adaptive_placementpb.DatasetStats,
	now int64,
) *adaptive_placementpb.DatasetPlacement {
	if ds.defaultLoadBalancing != DynamicLoadBalancing {
		ds.loadBalancing = ds.defaultLoadBalancing.proto()
	} else if ds.defaultLoadBalancing.needsDynamicBalancing(ds.loadBalancing) {
		ds.loadBalancing = loadBalancingStrategy(stats, ds.allocator.unitSize).proto()
	}
	shards := uint32(ds.allocator.observe(sum(stats.Usage), now))
	return &adaptive_placementpb.DatasetPlacement{
		Tenant:        stats.Tenant,
		Name:          stats.Name,
		ShardLimit:    shards,
		LoadBalancing: ds.loadBalancing,
	}
}
