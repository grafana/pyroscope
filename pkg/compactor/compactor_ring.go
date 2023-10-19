// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/compactor_ring.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"flag"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"

	"github.com/grafana/pyroscope/pkg/util"
)

const (
	// ringNumTokens is how many tokens each compactor should have in the ring. We use a
	// safe default instead of exposing to config option to the user in order to simplify
	// the config.
	ringNumTokens = 512
)

// RingConfig masks the ring lifecycler config which contains
// many options not really required by the compactors ring. This config
// is used to strip down the config to the minimum, and avoid confusion
// to the user.
type RingConfig struct {
	Common util.CommonRingConfig `yaml:",inline"`

	// Wait ring stability.
	WaitStabilityMinDuration time.Duration `yaml:"wait_stability_min_duration" category:"advanced"`
	WaitStabilityMaxDuration time.Duration `yaml:"wait_stability_max_duration" category:"advanced"`

	WaitActiveInstanceTimeout time.Duration `yaml:"wait_active_instance_timeout" category:"advanced"`

	ObservePeriod time.Duration `yaml:"-"`
}

// RegisterFlags adds the flags required to config this to the given FlagSet
func (cfg *RingConfig) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	const flagNamePrefix = "compactor.ring."
	const kvStorePrefix = "collectors/"
	const componentPlural = "compactors"
	cfg.Common.RegisterFlags(flagNamePrefix, kvStorePrefix, componentPlural, f, logger)

	// Wait stability flags.
	f.DurationVar(&cfg.WaitStabilityMinDuration, flagNamePrefix+"wait-stability-min-duration", 0, "Minimum time to wait for ring stability at startup. 0 to disable.")
	f.DurationVar(&cfg.WaitStabilityMaxDuration, flagNamePrefix+"wait-stability-max-duration", 5*time.Minute, "Maximum time to wait for ring stability at startup. If the compactor ring keeps changing after this period of time, the compactor will start anyway.")

	// Timeout durations
	f.DurationVar(&cfg.WaitActiveInstanceTimeout, flagNamePrefix+"wait-active-instance-timeout", 10*time.Minute, "Timeout for waiting on compactor to become ACTIVE in the ring.")
}

func (cfg *RingConfig) ToBasicLifecyclerConfig(logger log.Logger) (ring.BasicLifecyclerConfig, error) {
	instanceAddr, err := ring.GetInstanceAddr(cfg.Common.InstanceAddr, cfg.Common.InstanceInterfaceNames, logger, cfg.Common.EnableIPv6)
	if err != nil {
		return ring.BasicLifecyclerConfig{}, err
	}

	instancePort := ring.GetInstancePort(cfg.Common.InstancePort, cfg.Common.ListenPort)

	return ring.BasicLifecyclerConfig{
		ID:                              cfg.Common.InstanceID,
		Addr:                            fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		HeartbeatPeriod:                 cfg.Common.HeartbeatPeriod,
		HeartbeatTimeout:                cfg.Common.HeartbeatTimeout,
		TokensObservePeriod:             cfg.ObservePeriod,
		NumTokens:                       ringNumTokens,
		KeepInstanceInTheRingOnShutdown: false,
	}, nil
}

func (cfg *RingConfig) toRingConfig() ring.Config {
	rc := cfg.Common.ToRingConfig()
	rc.ReplicationFactor = 1

	return rc
}
