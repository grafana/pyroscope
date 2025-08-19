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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="normalized",tenant="tenant-received"} 980`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-received"} 990`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="received",tenant="tenant-received"} 1000`,
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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="normalized",tenant="tenant-sampled"} 1980`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-sampled"} 2000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="sampled",tenant="tenant-sampled"} 1990`,
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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-normalized"} 3000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-normalized"} 2990`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="normalized",tenant="tenant-normalized"} 2980`,
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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="normalized",tenant="tenant-no-norm"} 3990`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-no-norm"} 4000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="sampled",tenant="tenant-no-norm"} 3990`,
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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-no-sampled"} 5000`,
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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-norm-not-normalized"} 6000`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-norm-not-normalized"} 5990`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="normalized",tenant="tenant-norm-not-normalized"} 5990`,
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
