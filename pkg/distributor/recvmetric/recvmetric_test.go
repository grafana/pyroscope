package recvmetric

import (
	"regexp"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestMetricWithDifferentTenantStages(t *testing.T) {

	testCases := []struct {
		name            string
		tenant          string
		tenantStage     Stage
		receivedSize    uint64
		sampled         bool
		normalized      bool
		expectedMetrics []string
	}{
		{
			name:         "tenant with received stage",
			tenant:       "tenant-received",
			tenantStage:  StageReceived,
			receivedSize: 1000,
			sampled:      true,
			normalized:   true,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="tenant-received",tenant_stage="false"} 980`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="tenant-received",tenant_stage="true"} 1000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="tenant-received",tenant_stage="false"} 990`,
			},
		},
		{
			name:         "tenant with sampled stage",
			tenant:       "tenant-sampled",
			tenantStage:  StageSampled,
			receivedSize: 2000,
			sampled:      true,
			normalized:   true,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="tenant-sampled",tenant_stage="false"} 1980`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="tenant-sampled",tenant_stage="false"} 2000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="tenant-sampled",tenant_stage="true"} 1990`,
			},
		},
		{
			name:         "tenant with normalized stage",
			tenant:       "tenant-normalized",
			tenantStage:  StageNormalized,
			receivedSize: 3000,
			sampled:      true,
			normalized:   true,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="tenant-normalized",tenant_stage="true"} 2980`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="tenant-normalized",tenant_stage="false"} 3000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="tenant-normalized",tenant_stage="false"} 2990`,
			},
		},
		{
			name:         "tenant with sampled stage but no normalization",
			tenant:       "tenant-no-norm",
			tenantStage:  StageSampled,
			receivedSize: 4000,
			sampled:      true,
			normalized:   false,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="tenant-no-norm",tenant_stage="false"} 3990`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="tenant-no-norm",tenant_stage="false"} 4000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="tenant-no-norm",tenant_stage="true"} 3990`,
			},
		},
		{
			name:         "tenant without sampled stage recorded",
			tenant:       "tenant-no-sampled",
			tenantStage:  StageSampled,
			receivedSize: 5000,
			sampled:      false,
			normalized:   false,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="tenant-no-sampled",tenant_stage="false"} 5000`,
			},
		},
		{
			name:         "tenant with normalized stage but not normalized",
			tenant:       "tenant-norm-not-normalized",
			tenantStage:  StageNormalized,
			receivedSize: 6000,
			sampled:      true,
			normalized:   false,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="tenant-norm-not-normalized",tenant_stage="true"} 5990`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="tenant-norm-not-normalized",tenant_stage="false"} 6000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="tenant-norm-not-normalized",tenant_stage="false"} 5990`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := New(reg)
			req := m.NewRequest(tc.tenant, tc.tenantStage, tc.receivedSize)

			if tc.sampled {
				sampledSize := tc.receivedSize - 10
				req.Record(StageSampled, sampledSize)

				if tc.normalized {
					normalizedSize := sampledSize - 10
					req.Record(StageNormalized, normalizedSize)
				}
			}

			req.Observe()

			bs, err := testutil.CollectAndFormat(reg, expfmt.TypeTextPlain, NamespacedMetricName)
			require.NoError(t, err)

			sums := regexp.MustCompile(NamespacedMetricName+"_sum.*").
				FindAllString(string(bs), -1) // do not depend on buckets

			require.Equal(t, tc.expectedMetrics, sums)
		})
	}
}

func TestRecordPanic(t *testing.T) {
	m := New(nil)
	req := m.NewRequest("test-tenant", StageSampled, 1000)

	require.Panics(t, func() {
		req.Record(StageReceived, 500)
	})

	require.NotPanics(t, func() {
		req.Record(StageSampled, 900)
		req.Record(StageNormalized, 800)
	})
}
