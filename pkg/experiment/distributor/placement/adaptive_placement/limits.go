package adaptive_placement

import (
	"time"
)

// The default values are only used if no limits are defined.
const (
	defaultTenantShardLimit  uint32 = 0 // Disabled.
	defaultDatasetShardLimit uint32 = 2
	defaultLoadBalancing            = FingerprintLoadBalancing
)

type Limits interface {
	ShardingLimits(tenant string) ShardingLimits
}

type ShardingLimits struct {
	TenantShards         uint32
	DefaultDatasetShards uint32
	MinDatasetShards     uint32
	MaxDatasetShards     uint32
	UnitSizeBytes        uint32
	BurstWindow          time.Duration
	DecayWindow          time.Duration
	LoadBalancing        LoadBalancing
}
