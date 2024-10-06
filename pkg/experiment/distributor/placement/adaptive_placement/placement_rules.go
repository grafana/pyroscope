package adaptive_placement

import (
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

type PlacementRulesBuilder struct {
	stats *StatsTracker
}

func (r *PlacementRulesBuilder) Build() *adaptive_placementpb.PlacementRules {
	return &adaptive_placementpb.PlacementRules{}
}
