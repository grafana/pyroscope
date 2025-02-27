// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingConfig_DefaultConfigToBasicLifecyclerConfig(t *testing.T) {
	cfg := Config{}
	flagext.DefaultValues(&cfg)
	cfg.SchedulerRing.InstanceAddr = "127.0.0.1"
	cfg.SchedulerRing.InstancePort = 9095

	expected := ring.BasicLifecyclerConfig{
		ID:                              cfg.SchedulerRing.InstanceID,
		Addr:                            net.JoinHostPort(cfg.SchedulerRing.InstanceAddr, strconv.Itoa(cfg.SchedulerRing.InstancePort)),
		HeartbeatPeriod:                 cfg.SchedulerRing.HeartbeatPeriod,
		HeartbeatTimeout:                cfg.SchedulerRing.HeartbeatTimeout,
		TokensObservePeriod:             0,
		NumTokens:                       1,
		KeepInstanceInTheRingOnShutdown: false,
	}

	actual, err := toBasicLifecyclerConfig(cfg.SchedulerRing, log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestRingConfig_CustomConfigToBasicLifecyclerConfig(t *testing.T) {
	// Customize the query-scheduler ring config
	cfg := Config{}
	flagext.DefaultValues(&cfg)
	cfg.SchedulerRing.HeartbeatPeriod = 1 * time.Second
	cfg.SchedulerRing.HeartbeatTimeout = 10 * time.Second
	cfg.SchedulerRing.InstanceID = "test"
	cfg.SchedulerRing.InstancePort = 10
	cfg.SchedulerRing.InstanceAddr = "1.2.3.4"
	cfg.SchedulerRing.ListenPort = 10

	// The lifecycler config should be generated based upon the query-scheduler
	// ring config
	expected := ring.BasicLifecyclerConfig{
		ID:                              "test",
		Addr:                            net.JoinHostPort(cfg.SchedulerRing.InstanceAddr, strconv.Itoa(cfg.SchedulerRing.InstancePort)),
		HeartbeatPeriod:                 1 * time.Second,
		HeartbeatTimeout:                10 * time.Second,
		TokensObservePeriod:             0,
		NumTokens:                       1,
		KeepInstanceInTheRingOnShutdown: false,
	}

	actual, err := toBasicLifecyclerConfig(cfg.SchedulerRing, log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestRingConfig_AddressFamilies(t *testing.T) {
	cfg := Config{}
	flagext.DefaultValues(&cfg)

	t.Run("IPv4", func(t *testing.T) {
		cfg.SchedulerRing.InstanceAddr = "1.2.3.4"
		cfg.SchedulerRing.InstancePort = 10
		actual, err := toBasicLifecyclerConfig(cfg.SchedulerRing, log.NewNopLogger())
		require.NoError(t, err)
		assert.Equal(t, "1.2.3.4:10", actual.Addr)
	})

	t.Run("IPv6", func(t *testing.T) {
		cfg.SchedulerRing.InstanceAddr = "::1"
		cfg.SchedulerRing.InstancePort = 10
		cfg.SchedulerRing.EnableIPv6 = true
		actual, err := toBasicLifecyclerConfig(cfg.SchedulerRing, log.NewNopLogger())
		require.NoError(t, err)
		assert.Equal(t, "[::1]:10", actual.Addr)
	})
}
