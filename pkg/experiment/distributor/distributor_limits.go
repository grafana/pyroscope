package distributor

import (
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
)

type Limits interface {
	PlacementPolicy(placement.Key) placement.Policy
}

var DefaultLimits = defaultLimits{}

type defaultLimits struct{}

func (defaultLimits) PlacementPolicy(k placement.Key) placement.Policy {
	return placement.Policy{
		TenantShards:  0, // Unlimited.
		DatasetShards: 1,
		PickShard: func(n int) int {
			return int(k.Fingerprint % uint64(n))
		},
	}
}
