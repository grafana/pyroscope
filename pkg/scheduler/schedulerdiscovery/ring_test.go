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
	cfg := RingConfig{}
	flagext.DefaultValues(&cfg)
	cfg.InstanceAddr = "127.0.0.1"
	cfg.InstancePort = 9095

	expected := ring.BasicLifecyclerConfig{
		ID:                              cfg.InstanceID,
		Addr:                            fmt.Sprintf("%s:%d", cfg.InstanceAddr, cfg.InstancePort),
		HeartbeatPeriod:                 cfg.HeartbeatPeriod,
		HeartbeatTimeout:                cfg.HeartbeatTimeout,
		TokensObservePeriod:             0,
		NumTokens:                       1,
		KeepInstanceInTheRingOnShutdown: false,
	}

	actual, err := cfg.ToBasicLifecyclerConfig(log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestRingConfig_CustomConfigToBasicLifecyclerConfig(t *testing.T) {
	// Customize the query-scheduler ring config
	cfg := RingConfig{}
	flagext.DefaultValues(&cfg)
	cfg.HeartbeatPeriod = 1 * time.Second
	cfg.HeartbeatTimeout = 10 * time.Second
	cfg.InstanceID = "test"
	cfg.InstancePort = 10
	cfg.InstanceAddr = "1.2.3.4"
	cfg.ListenPort = 10

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

	actual, err := cfg.ToBasicLifecyclerConfig(log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}
