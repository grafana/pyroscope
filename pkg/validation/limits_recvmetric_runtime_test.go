package validation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/distributor/recvmetric"
)

const distributorReceiveMetricStageOverrideConfig = `
overrides:
  no-overrides: {}
  tenant-received:
    distributor_receive_metric_stage: received
  tenant-sampled:
    distributor_receive_metric_stage: sampled
  tenant-normalized:
    distributor_receive_metric_stage: normalized
`

func Test_DistributorReceiveMetricStageInvalidConfig(t *testing.T) {
	invalidConfig := `
overrides:
  invalid-stage:
    distributor_receive_metric_stage: invalid
`
	_, err := LoadRuntimeConfig(bytes.NewReader([]byte(invalidConfig)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected Stage value")
}

func Test_DistributorReceiveMetricStageDifferentDefaults(t *testing.T) {
	testCases := []struct {
		name           string
		defaultStage   recvmetric.Stage
		expectedValues map[string]recvmetric.Stage
	}{
		{
			name:         "default received",
			defaultStage: recvmetric.StageReceived,
			expectedValues: map[string]recvmetric.Stage{
				"no-overrides":      recvmetric.StageReceived,
				"tenant-received":   recvmetric.StageReceived,
				"tenant-sampled":    recvmetric.StageSampled,
				"tenant-normalized": recvmetric.StageNormalized,
				"unknown-tenant":    recvmetric.StageReceived,
			},
		},
		{
			name:         "default normalized",
			defaultStage: recvmetric.StageNormalized,
			expectedValues: map[string]recvmetric.Stage{
				"no-overrides":      recvmetric.StageNormalized,
				"tenant-received":   recvmetric.StageReceived,
				"tenant-sampled":    recvmetric.StageSampled,
				"tenant-normalized": recvmetric.StageNormalized,
				"unknown-tenant":    recvmetric.StageNormalized,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDefaults := Limits{
				DistributorReceiveMetricStage: tc.defaultStage,
			}
			restore := SetDefaultLimitsForYAMLUnmarshalling(testDefaults)
			defer restore()

			rc, err := LoadRuntimeConfig(bytes.NewReader([]byte(distributorReceiveMetricStageOverrideConfig)))
			require.NoError(t, err)

			o, err := NewOverrides(testDefaults, &wrappedRuntimeConfig{rc})
			require.NoError(t, err)

			for tenant, expectedStage := range tc.expectedValues {
				actualStage := o.DistributorReceiveMetricStage(tenant)
				assert.Equal(t, expectedStage, actualStage,
					"Tenant %s should have stage %s but got %s",
					tenant, expectedStage.String(), actualStage.String())
			}
		})
	}
}
