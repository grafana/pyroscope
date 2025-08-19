package distributor

import (
	"context"
	"os"
	"regexp"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/distributor/recvmetric"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/testhelper"
	"github.com/grafana/pyroscope/pkg/validation"
)

func TestDistributorPushWithDifferentTenantStages(t *testing.T) {
	testCases := []struct {
		name            string
		tenant          string
		profilePath     string
		tenantStage     recvmetric.Stage
		limitOverrides  func(l *validation.Limits)
		failIngester    bool
		expectErr       bool
		expectedErrMsg  string
		expectedMetrics []string
	}{
		{
			name:        "tenant with received stage - successful push",
			tenant:      "tenant-received",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			tenantStage: recvmetric.StageReceived,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="normalized",tenant="tenant-received"} 2024`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-received"} 2144`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="received",tenant="tenant-received"} 2198`,
			},
		},
		{
			name:        "tenant with sampled stage - successful push",
			tenant:      "tenant-sampled",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			tenantStage: recvmetric.StageSampled,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="normalized",tenant="tenant-sampled"} 2024`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-sampled"} 2198`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="sampled",tenant="tenant-sampled"} 2144`,
			},
		},
		{
			name:        "tenant with normalized stage - successful push",
			tenant:      "tenant-normalized",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			tenantStage: recvmetric.StageNormalized,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-normalized"} 2198`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-normalized"} 2144`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="normalized",tenant="tenant-normalized"} 2024`,
			},
		},
		{
			name:        "tenant with rate limit - only received stage",
			tenant:      "tenant-rate-limited",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			tenantStage: recvmetric.StageSampled,
			limitOverrides: func(l *validation.Limits) {
				l.IngestionRateMB = 0.000001
				l.IngestionBurstSizeMB = 0.000001
			},
			expectErr:      true,
			expectedErrMsg: "rate limit",
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-rate-limited"} 2198`,
			},
		},
		{
			name:         "tenant with normalized stage but ingester fails",
			tenant:       "tenant-ingester-fail",
			profilePath:  "../../pkg/og/convert/testdata/cpu.pprof",
			tenantStage:  recvmetric.StageNormalized,
			failIngester: true,
			expectErr:    true,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-ingester-fail"} 2198`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-ingester-fail"} 2144`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="normalized",tenant="tenant-ingester-fail"} 2024`,
			},
		},
		{
			name:        "heap profile with sample type relabeling - keep only inuse_space",
			tenant:      "tenant-heap-relabel",
			profilePath: "../../pkg/pprof/testdata/heap",
			tenantStage: recvmetric.StageNormalized,
			limitOverrides: func(l *validation.Limits) {
				l.SampleTypeRelabelingRules = []*relabel.Config{
					{
						SourceLabels: []model.LabelName{"__type__"},
						Regex:        relabel.MustNewRegexp("inuse_space"),
						Action:       relabel.Keep,
					},
				}
			},
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="received",tenant="tenant-heap-relabel"} 847192`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="false",stage="sampled",tenant="tenant-heap-relabel"} 847138`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{is_tenant_stage="true",stage="normalized",tenant="tenant-heap-relabel"} 46234`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			ing := newFakeIngester(t, tc.failIngester)

			overrides := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
				l := validation.MockDefaultLimits()
				l.DistributorReceiveMetricStage = tc.tenantStage

				if tc.limitOverrides != nil {
					tc.limitOverrides(l)
				}

				tenantLimits[tc.tenant] = l
			})

			d, err := New(
				Config{DistributorRing: ringConfig},
				testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
				&poolFactory{func(addr string) (client.PoolClient, error) {
					return ing, nil
				}},
				overrides,
				reg,
				log.NewLogfmtLogger(os.Stdout),
				nil,
			)
			require.NoError(t, err)

			profileBytes, err := os.ReadFile(tc.profilePath)
			require.NoError(t, err)

			parsedProfile, err := pprof.RawFromBytes(profileBytes)
			require.NoError(t, err)

			ctx := tenant.InjectTenantID(context.Background(), tc.tenant)
			req := &distributormodel.PushRequest{
				Series: []*distributormodel.ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "cluster", Value: "test-cluster"},
							{Name: phlaremodel.LabelNameServiceName, Value: "test-service"},
							{Name: "__name__", Value: "cpu"},
						},
						RawProfile: profileBytes,
						Profile:    parsedProfile,
					},
				},
				RawProfileType: distributormodel.RawProfileTypePPROF,
			}

			err = d.PushBatch(ctx, req)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrMsg != "" {
					require.Contains(t, err.Error(), tc.expectedErrMsg)
				}
			} else {
				require.NoError(t, err)
			}

			bs, err := testutil.CollectAndFormat(reg, expfmt.TypeTextPlain, recvmetric.NamespacedMetricName)
			require.NoError(t, err)

			sums := regexp.MustCompile(recvmetric.NamespacedMetricName+`_sum.*`).
				FindAllString(string(bs), -1)

			assert.Equal(t, tc.expectedMetrics, sums)
		})
	}
}
