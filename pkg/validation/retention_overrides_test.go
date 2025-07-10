package validation

import (
	"bytes"
	"flag"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/metastore/index/cleaner/retention"
)

func TestRetentionOverridesYAML(t *testing.T) {
	yamlConfig := `
overrides:
  tenant-default: {}
  tenant-short:
    retention_period: 24h
  tenant-long:
    retention_period: 30d
  tenant-infinite:
    retention_period: 0
`

	runtimeConfig, err := LoadRuntimeConfig(bytes.NewReader([]byte(yamlConfig)))
	require.NoError(t, err)

	var testLimits Limits
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	testLimits.RegisterFlags(fs)
	testLimits.Retention.RegisterFlags(fs)
	expectedDefault := model.Duration(31 * 24 * time.Hour)

	overrides, err := NewOverrides(testLimits, &mockTenantLimits{
		limits: runtimeConfig.TenantLimits,
		config: runtimeConfig,
	})
	require.NoError(t, err)
	defaults, overridesIter := overrides.Retention()
	assert.Equal(t, expectedDefault, defaults.RetentionPeriod)

	tenantOverrides := make(map[string]retention.Config)
	for tenantID, config := range overridesIter {
		tenantOverrides[tenantID] = config
	}

	expectedOverrides := map[string]model.Duration{
		"tenant-short":    model.Duration(24 * time.Hour),
		"tenant-long":     model.Duration(720 * time.Hour),
		"tenant-infinite": 0, // Infinite retention.
	}

	for tenantID, expectedPeriod := range expectedOverrides {
		config, exists := tenantOverrides[tenantID]
		require.True(t, exists, "tenant %s should have override config", tenantID)
		assert.Equal(t, expectedPeriod, config.RetentionPeriod,
			"tenant %s should have retention period %v", tenantID, expectedPeriod)
	}
}

func TestRetentionOverrides(t *testing.T) {
	expectedDefault := model.Duration(31 * 24 * time.Hour)

	tests := []struct {
		name          string
		tenantLimits  map[string]*Limits
		wantOverrides map[string]model.Duration
	}{
		{
			name:          "no overrides",
			tenantLimits:  nil,
			wantOverrides: map[string]model.Duration{},
		},
		{
			name: "with tenant overrides",
			tenantLimits: map[string]*Limits{
				"tenant-a": {Retention: retention.Config{RetentionPeriod: model.Duration(30 * 24 * time.Hour)}},
				"tenant-b": {Retention: retention.Config{RetentionPeriod: 0}}, // infinite
			},
			wantOverrides: map[string]model.Duration{
				"tenant-a": model.Duration(30 * 24 * time.Hour),
				"tenant-b": 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var testLimits Limits
			fs := flag.NewFlagSet("test", flag.PanicOnError)
			testLimits.RegisterFlags(fs)
			testLimits.Retention.RegisterFlags(fs)

			var tenantLimits TenantLimits
			if tt.tenantLimits != nil {
				tenantLimits = &mockTenantLimits{
					limits: tt.tenantLimits,
					config: &RuntimeConfigValues{TenantLimits: tt.tenantLimits},
				}
			}

			overrides, err := NewOverrides(testLimits, tenantLimits)
			require.NoError(t, err)

			defaults, overridesIter := overrides.Retention()
			assert.Equal(t, expectedDefault, defaults.RetentionPeriod)

			actualOverrides := make(map[string]model.Duration)
			for tenantID, config := range overridesIter {
				actualOverrides[tenantID] = config.RetentionPeriod
			}
			assert.Equal(t, tt.wantOverrides, actualOverrides)

			secondPass := make(map[string]model.Duration)
			for tenantID, config := range overridesIter {
				secondPass[tenantID] = config.RetentionPeriod
			}
			assert.Equal(t, actualOverrides, secondPass)
		})
	}
}
