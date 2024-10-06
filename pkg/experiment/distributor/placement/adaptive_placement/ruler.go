package adaptive_placement

import (
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type Ruler struct {
	stats    *DistributionStats
	limits   Limits
	datasets map[datasetKey]*datasetShards
	rules    *adaptive_placementpb.PlacementRules
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
				loadBalancing: limits.LoadBalancing,
				allocator: &shardAllocator{
					min:         limits.MinDatasetShards,
					max:         limits.MaxDatasetShards,
					unitSize:    limits.UnitSizeBytes,
					burstWindow: limits.BurstWindow.Nanoseconds(),
					decayWindow: limits.DecayWindow.Nanoseconds(),
				},
			}
			r.datasets[k] = dataset
		}
		rules.Datasets[i] = dataset.placement(datasetStats, now)
	}

	return &rules
}

type datasetKey struct {
	tenant  string
	dataset string
}

type datasetShards struct {
	loadBalancing LoadBalancing
	allocator     *shardAllocator
}

func (ds *datasetShards) placement(
	stats *adaptive_placementpb.DatasetStats,
	now int64,
) *adaptive_placementpb.DatasetPlacement {
	var sum uint64
	for _, u := range stats.Usage {
		sum += u
	}
	shards := uint32(ds.allocator.observe(sum, now))
	loadbalancing := ds.loadBalancing
	if loadbalancing == DynamicLoadBalancing {
		loadbalancing = selectLoadBalancing(stats, ds.allocator.unitSize)
	}
	return &adaptive_placementpb.DatasetPlacement{
		Tenant:        stats.Tenant,
		Name:          stats.Name,
		ShardLimit:    shards,
		LoadBalancing: loadbalancing.proto(),
	}
}
