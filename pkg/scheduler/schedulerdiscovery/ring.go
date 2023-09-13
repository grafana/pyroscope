// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

const (
	// ringKey is the key under which we store the query-schedulers ring in the KVStore.
	ringKey = "query-scheduler"

	// ringNumTokens is how many tokens each query-scheduler should have in the ring.
	// Query-schedulers use a ring for service-discovery so just 1 token is enough.
	ringNumTokens = 1

	// ringAutoForgetUnhealthyPeriods is how many consecutive timeout periods an unhealthy instance
	// in the ring will be automatically removed after.
	ringAutoForgetUnhealthyPeriods = 4

	// sharedOptionWithRingClient is a message appended to all config options that should be also
	// set on the components running the query-scheduler ring client.
	sharedOptionWithRingClient = " When query-scheduler ring-based service discovery is enabled, this option needs be set on query-schedulers, query-frontends and queriers."
)

// toBasicLifecyclerConfig returns a ring.BasicLifecyclerConfig based on the query-scheduler ring config.
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

// NewRingLifecycler creates a new query-scheduler ring lifecycler with all required lifecycler delegates.
func NewRingLifecycler(cfg util.CommonRingConfig, logger log.Logger, reg prometheus.Registerer) (*ring.BasicLifecycler, error) {
	reg = prometheus.WrapRegistererWithPrefix("pyroscope_", reg)
	kvStore, err := kv.NewClient(cfg.KVStore, ring.GetCodec(), kv.RegistererWithKVName(reg, "query-scheduler-lifecycler"), logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize query-schedulers' KV store")
	}

	lifecyclerCfg, err := toBasicLifecyclerConfig(cfg, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build query-schedulers' lifecycler config")
	}

	var delegate ring.BasicLifecyclerDelegate
	delegate = ring.NewInstanceRegisterDelegate(ring.ACTIVE, ringNumTokens)
	delegate = ring.NewLeaveOnStoppingDelegate(delegate, logger)
	delegate = ring.NewAutoForgetDelegate(ringAutoForgetUnhealthyPeriods*cfg.HeartbeatTimeout, delegate, logger)

	lifecycler, err := ring.NewBasicLifecycler(lifecyclerCfg, "query-scheduler", ringKey, kvStore, delegate, logger, reg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize query-schedulers' lifecycler")
	}

	return lifecycler, nil
}

// NewRingClient creates a client for the query-schedulers ring.
func NewRingClient(cfg util.CommonRingConfig, component string, logger log.Logger, reg prometheus.Registerer) (*ring.Ring, error) {
	client, err := ring.New(cfg.ToRingConfig(), component, ringKey, logger, prometheus.WrapRegistererWithPrefix("pyroscope_", reg))
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize query-schedulers' ring client")
	}

	return client, err
}
