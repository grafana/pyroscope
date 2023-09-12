// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"fmt"
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
		Addr:                            fmt.Sprintf("%s:%d", cfg.SchedulerRing.InstanceAddr, cfg.SchedulerRing.InstancePort),
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
		Addr:                            "1.2.3.4:10",
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
