package adaptive_placement

import (
	"go.uber.org/atomic"

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
	datasets atomic.Pointer[map[datasetKey]*adaptive_placementpb.DatasetPlacement]
	limits   Limits
}

func NewAdaptivePlacement(limits Limits) *AdaptivePlacement {
	return &AdaptivePlacement{limits: limits}
}

func (a *AdaptivePlacement) Policy(k placement.Key) placement.Policy {
	dk := datasetKey{
		tenant:  k.TenantID,
		dataset: k.DatasetName,
	}
	datasets := a.datasets.Load()
	if datasets == nil {
		return a.defaultPolicy(k)
	}
	dataset, ok := (*datasets)[dk]
	if !ok {
		return a.defaultPolicy(k)
	}
	return placement.Policy{
		TenantShards:  int(dataset.TenantShardLimit),
		DatasetShards: int(dataset.DatasetShardLimit),
		PickShard:     loadBalancingFromProto(dataset.LoadBalancing).pick(k),
	}
}

func (a *AdaptivePlacement) defaultPolicy(k placement.Key) placement.Policy {
	limits := a.limits.PlacementLimits(k.TenantID)
	return placement.Policy{
		TenantShards:  int(limits.TenantShards),
		DatasetShards: int(limits.DefaultDatasetShards),
		PickShard:     limits.LoadBalancing.pick(k),
	}
}

func (a *AdaptivePlacement) Update(rules *adaptive_placementpb.PlacementRules) {
	datasets := make(map[datasetKey]*adaptive_placementpb.DatasetPlacement, len(rules.Datasets))
	for _, dataset := range rules.Datasets {
		k := datasetKey{
			tenant:  rules.Tenants[dataset.Tenant].TenantId,
			dataset: dataset.Name,
		}
		datasets[k] = dataset
	}
	a.datasets.Store(&datasets)
}
