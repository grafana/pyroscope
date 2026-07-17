package distributor

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/distributor/ingestlimits"
	distributormodel "github.com/grafana/pyroscope/v2/pkg/distributor/model"
	"github.com/grafana/pyroscope/v2/pkg/distributor/sampling"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	pprof2 "github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestDistributor_ScopeUsage(t *testing.T) {
	const scopeName = "com.grafana.pyroscope/test-scope"

	var logs bytes.Buffer
	d, _, err := newTestDistributor(t, log.NewLogfmtLogger(&logs), validation.MockDefaultOverrides())
	require.NoError(t, err)

	scopeUsageBefore, _ := scopeUsageTotal(t, d, scopeName)
	languageUsageBefore := multiCounterTotal(t, d.profileReceivedStats.Value())

	for _, push := range []struct {
		tenantID     string
		scopeName    string
		scopeVersion string
	}{
		{tenantID: "tenant-a", scopeName: scopeName, scopeVersion: "1.2.3"},
		{tenantID: "tenant-a", scopeName: scopeName, scopeVersion: "1.2.3"},
		{tenantID: "tenant-a", scopeName: scopeName},
		{tenantID: "tenant-b", scopeName: scopeName, scopeVersion: "1.2.3"},
		{tenantID: "tenant-a", scopeVersion: "version-without-name"},
	} {
		require.NoError(t, pushScopeProfile(t, d, push.tenantID, push.scopeName, push.scopeVersion))
	}

	require.NoError(t, testutil.CollectAndCompare(
		d.metrics.profilesReceived,
		strings.NewReader(`# HELP pyroscope_distributor_profiles_received_total The total number of profiles received by the distributor, broken down by OpenTelemetry instrumentation scope.
# TYPE pyroscope_distributor_profiles_received_total counter
pyroscope_distributor_profiles_received_total{scope_name="com.grafana.pyroscope/test-scope",scope_version="",tenant="tenant-a"} 1
pyroscope_distributor_profiles_received_total{scope_name="com.grafana.pyroscope/test-scope",scope_version="1.2.3",tenant="tenant-a"} 2
pyroscope_distributor_profiles_received_total{scope_name="com.grafana.pyroscope/test-scope",scope_version="1.2.3",tenant="tenant-b"} 1
`),
		"pyroscope_distributor_profiles_received_total",
	))

	scopeUsageAfter, scopeEntry := scopeUsageTotal(t, d, scopeName)
	require.Equal(t, int64(4), scopeUsageAfter-scopeUsageBefore)
	require.NotContains(t, scopeEntry, "tenant")
	require.NotContains(t, scopeEntry, "scope_version")
	require.Equal(t, int64(5), multiCounterTotal(t, d.profileReceivedStats.Value())-languageUsageBefore)

	logOutput := logs.String()
	require.Equal(t, 5, strings.Count(logOutput, `msg="profile accepted"`))
	require.Contains(t, logOutput, "otel_scope_name="+scopeName)
	require.Contains(t, logOutput, "otel_scope_version=1.2.3")
	require.Contains(t, logOutput, "otel_scope_version=version-without-name")
}

func TestDistributor_ScopeUsageNotCountedBeforeExistingUsagePoint(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		expectErr bool
		configure func(t *testing.T, limits *validation.Limits)
	}{
		{
			name:      "ingest limit",
			message:   `msg="rejecting profile due to global ingest limit"`,
			expectErr: true,
			configure: func(_ *testing.T, limits *validation.Limits) {
				limits.IngestionLimit = &ingestlimits.Config{
					PeriodType:     "hour",
					PeriodLimitMb:  128,
					LimitResetTime: time.Now().Add(time.Hour).Unix(),
					LimitReached:   true,
					Sampling: ingestlimits.SamplingConfig{
						NumRequests: 0,
						Period:      time.Minute,
					},
				}
			},
		},
		{
			name:    "sampling",
			message: `msg="skipping profile due to sampling"`,
			configure: func(t *testing.T, limits *validation.Limits) {
				usageGroups, err := validation.NewUsageGroupConfig(map[string]string{
					"all": `{service_name="test-service"}`,
				})
				require.NoError(t, err)
				limits.DistributorUsageGroups = usageGroups
				limits.DistributorSampling = &sampling.Config{
					UsageGroups: map[string]sampling.UsageGroupSampling{
						"all": {Probability: 0},
					},
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testID := strings.ReplaceAll(test.name, " ", "-")
			tenantID := "tenant-" + testID
			scopeName := "com.grafana.pyroscope/test-" + testID
			overrides := validation.MockOverrides(func(_ *validation.Limits, tenantLimits map[string]*validation.Limits) {
				limits := validation.MockDefaultLimits()
				test.configure(t, limits)
				tenantLimits[tenantID] = limits
			})

			var logs bytes.Buffer
			d, _, err := newTestDistributor(t, log.NewLogfmtLogger(&logs), overrides)
			require.NoError(t, err)
			scopeUsageBefore, _ := scopeUsageTotal(t, d, scopeName)
			languageUsageBefore := multiCounterTotal(t, d.profileReceivedStats.Value())

			err = pushScopeProfile(t, d, tenantID, scopeName, "1.0.0")
			if test.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Zero(t, testutil.ToFloat64(d.metrics.profilesReceived.WithLabelValues(tenantID, scopeName, "1.0.0")))
			scopeUsageAfter, _ := scopeUsageTotal(t, d, scopeName)
			require.Equal(t, scopeUsageBefore, scopeUsageAfter)
			require.Equal(t, languageUsageBefore, multiCounterTotal(t, d.profileReceivedStats.Value()))
			require.Contains(t, logs.String(), test.message)
			require.Contains(t, logs.String(), "otel_scope_name="+scopeName)
			require.Contains(t, logs.String(), "otel_scope_version=1.0.0")
		})
	}
}

func pushScopeProfile(t *testing.T, d *Distributor, tenantID, scopeName, scopeVersion string) error {
	t.Helper()

	profileBytes := collectTestProfileBytes(t)
	profile, err := pprof2.RawFromBytes(profileBytes)
	require.NoError(t, err)

	labels := []*typesv1.LabelPair{
		{Name: phlaremodel.LabelNameProfileName, Value: "process_cpu"},
		{Name: phlaremodel.LabelNameServiceName, Value: "test-service"},
	}
	if scopeName != "" {
		labels = append(labels, &typesv1.LabelPair{Name: phlaremodel.LabelNameOTELScopeName, Value: scopeName})
	}
	if scopeVersion != "" {
		labels = append(labels, &typesv1.LabelPair{Name: phlaremodel.LabelNameOTELScopeVersion, Value: scopeVersion})
	}

	return d.PushBatch(tenant.InjectTenantID(context.Background(), tenantID), &distributormodel.PushRequest{
		RawProfileType: distributormodel.RawProfileTypePPROF,
		Series: []*distributormodel.ProfileSeries{
			{
				Labels:     labels,
				Profile:    profile,
				RawProfile: profileBytes,
			},
		},
	})
}

func scopeUsageTotal(t *testing.T, d *Distributor, scopeName string) (int64, map[string]interface{}) {
	t.Helper()

	drilldown, ok := d.profileScopeStats.Value()["drilldown"].([]interface{})
	require.True(t, ok)
	for _, value := range drilldown {
		entry, ok := value.(map[string]interface{})
		require.True(t, ok)
		if entry["scope"] != scopeName {
			continue
		}
		data, ok := entry["data"].(map[string]interface{})
		require.True(t, ok)
		total, ok := data["total"].(int64)
		require.True(t, ok)
		return total, entry
	}
	return 0, nil
}

func multiCounterTotal(t *testing.T, value map[string]interface{}) int64 {
	t.Helper()

	total, ok := value["total"].(int64)
	require.True(t, ok)
	return total
}
