package distributor

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"

	"github.com/grafana/pyroscope/pkg/util"
)

const (
	// ringNumTokens is how many tokens each distributor should have in the ring.
	// Distributors use a ring because they need to know how many distributors there
	// are in total for rate limiting.
	ringNumTokens = 1
)

func toBasicLifecyclerConfig(cfg util.CommonRingConfig, logger log.Logger) (ring.BasicLifecyclerConfig, error) {
	instanceAddr, err := ring.GetInstanceAddr(cfg.InstanceAddr, cfg.InstanceInterfaceNames, logger, false)
	if err != nil {
		return ring.BasicLifecyclerConfig{}, err
	}

	instancePort := ring.GetInstancePort(cfg.InstancePort, cfg.ListenPort)

	return ring.BasicLifecyclerConfig{
		ID:                              cfg.InstanceID,
		Addr:                            fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		HeartbeatPeriod:                 cfg.HeartbeatPeriod,
		HeartbeatTimeout:                cfg.HeartbeatTimeout,
		TokensObservePeriod:             0,
		NumTokens:                       ringNumTokens,
		KeepInstanceInTheRingOnShutdown: false,
	}, nil
}
