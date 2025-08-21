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
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/testhelper"
	"github.com/grafana/pyroscope/pkg/validation"
)

func TestDistributorPushWithDifferentTenantStages(t *testing.T) {
	const tenantId = "239"

	testCases := []struct {
		name            string
		profilePath     string
		limitOverrides  func(l *validation.Limits)
		failIngester    bool
		expectErr       bool
		expectedErrMsg  string
		expectedMetrics []string
	}{
		{
			name:        "successful push",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="239"} 2024`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="239"} 2198`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="239"} 2144`,
			},
		},
		{
			name:        "rate limit - only received stage",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			limitOverrides: func(l *validation.Limits) {
				l.IngestionRateMB = 0.000001
				l.IngestionBurstSizeMB = 0.000001
			},
			expectErr:      true,
			expectedErrMsg: "rate limit",
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="239"} 2198`,
			},
		},

		{
			name:        "invalid profile",
			profilePath: "../../pkg/og/convert/testdata/cpu.pprof",
			limitOverrides: func(l *validation.Limits) {
				l.MaxProfileSizeBytes = 2
			},
			expectErr:      true,
			expectedErrMsg: "exceeds the size limit (max_profile_size_byte, actual: 2144, limit: 2)",
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="239"} 2198`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="239"} 2144`,
			},
		},
		{
			name:         "ingester fails",
			profilePath:  "../../pkg/og/convert/testdata/cpu.pprof",
			failIngester: true,
			expectErr:    true,
			expectedMetrics: []string{
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="239"} 2024`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="239"} 2198`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="239"} 2144`,
			},
		},
		{
			name:        "heap profile with sample type relabeling - keep only inuse_space",
			profilePath: "../../pkg/pprof/testdata/heap",
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
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="normalized",tenant="239"} 46234`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="received",tenant="239"} 847192`,
				`pyroscope_distributor_received_decompressed_bytes_total_sum{stage="sampled",tenant="239"} 847138`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			ing := newFakeIngester(t, tc.failIngester)

			overrides := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
				l := validation.MockDefaultLimits()

				if tc.limitOverrides != nil {
					tc.limitOverrides(l)
				}

				tenantLimits[tenantId] = l
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

			ctx := tenant.InjectTenantID(context.Background(), tenantId)
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

			bs, err := testutil.CollectAndFormat(reg, expfmt.TypeTextPlain, "pyroscope_distributor_received_decompressed_bytes_total")
			require.NoError(t, err)

			sums := regexp.MustCompile("pyroscope_distributor_received_decompressed_bytes_total_sum.*").
				FindAllString(string(bs), -1)

			assert.Equal(t, tc.expectedMetrics, sums)
		})
	}
}
