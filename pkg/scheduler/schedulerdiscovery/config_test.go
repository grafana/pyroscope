// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"testing"

	"github.com/grafana/dskit/flagext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		setup       func(cfg *Config)
		expectedErr string
	}{
		"should pass with default config": {
			setup: func(cfg *Config) {},
		},
		"should fail if service discovery mode is invalid": {
			setup: func(cfg *Config) {
				cfg.Mode = "xxx"
			},
			expectedErr: "unsupported query-scheduler service discovery mode",
		},
		"should fail if service discovery mode is set to DNS and max used instances has been configured": {
			setup: func(cfg *Config) {
				cfg.Mode = ModeDNS
				cfg.MaxUsedInstances = 1
			},
			expectedErr: "the query-scheduler max used instances can be set only when -query-scheduler.service-discovery-mode is set to 'ring'",
		},
		"should pass if service discovery mode is set to ring and max used instances has been configured": {
			setup: func(cfg *Config) {
				cfg.Mode = ModeRing
				cfg.MaxUsedInstances = 1
			},
		},
		"should fail if service discovery mode is set to ring but max used instances is negative": {
			setup: func(cfg *Config) {
				cfg.Mode = ModeRing
				cfg.MaxUsedInstances = -1
			},
			expectedErr: "the query-scheduler max used instances can't be negative",
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			cfg := Config{}
			flagext.DefaultValues(&cfg)
			testData.setup(&cfg)

			actualErr := cfg.Validate()
			if testData.expectedErr == "" {
				require.NoError(t, actualErr)
			} else {
				require.Error(t, actualErr)
				assert.ErrorContains(t, actualErr, testData.expectedErr)
			}
		})
	}
}
