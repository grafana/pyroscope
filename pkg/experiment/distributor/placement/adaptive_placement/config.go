package adaptive_placement

import (
	"time"
)

type Limits interface {
	ShardingLimits(tenant string) ShardingLimits
}

type ShardingLimits struct {
	TenantShards         uint32
	DefaultDatasetShards uint32
	LoadBalancing        LoadBalancing

	MinDatasetShards uint32
	MaxDatasetShards uint32
	UnitSizeBytes    uint32
	BurstWindow      time.Duration
	DecayWindow      time.Duration
}

type Config struct {
	PlacementUpdateInterval   time.Duration
	PlacementConfidencePeriod time.Duration
	PlacementRetentionPeriod  time.Duration

	StatsUpdateInterval    time.Duration
	StatsAggregationWindow time.Duration
	StatsRetentionPeriod   time.Duration
}
