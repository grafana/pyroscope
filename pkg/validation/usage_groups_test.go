package validation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func TestOverrides_DistributorUsageGroups(t *testing.T) {
	tests := []struct {
		Name           string
		TenantID       string
		UsageGroups    []map[string]string
		Labels         phlaremodel.Labels
		WantUsageGroup string
		WantErrMsg     string
	}{
		{
			Name:           "no_usage_groups",
			TenantID:       "tenant1",
			UsageGroups:    []map[string]string{},
			Labels:         phlaremodel.Labels{{Name: "service_name", Value: "foo"}},
			WantUsageGroup: "other",
		},
		{
			Name:     "single_matcher",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name="foo"}`},
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
				{Name: "namespace", Value: "foo_namespace"},
			},
			WantUsageGroup: "app/foo",
		},
		{
			Name:     "multiple_matchers",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name="foo", namespace="foo_namespace"}`},
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
				{Name: "namespace", Value: "foo_namespace"},
			},
			WantUsageGroup: "app/foo",
		},
		{
			Name:     "single_matcher_no_match",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name="foo"}`},
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "bar"},
			},
			WantUsageGroup: "other",
		},
		{
			Name:     "multiple_matchers_no_match",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name="foo", namespace="foo_namespace"}`},
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
				{Name: "namespace", Value: "bar_namespace"},
			},
			WantUsageGroup: "other",
		},
		{
			Name:     "multiple_usage_groups_match",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/almost_foo": `{service_name=~"foo.*", namespace="foo_namespace"}`},
				{"app/foo": `{service_name="foo"}`},
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
				{Name: "namespace", Value: "foo_namespace"},
			},
			WantUsageGroup: "app/foo",
		},
		{
			Name:     "match_everything_matcher",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{}`},
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "does_not_matter"},
			},
			WantUsageGroup: "app/foo",
		},
		{
			Name:     "no_labels",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name="foo"}`},
			},
			Labels:         phlaremodel.Labels{},
			WantUsageGroup: "other",
		},
		{
			Name:     "too_many_usage_groups",
			TenantID: "tenant1",
			UsageGroups: testGenerateUsageGroups(maxUsageGroups+1, map[string]string{
				"app/foo": `{service_name="foo"}`,
			}),
			WantErrMsg: fmt.Sprintf(`too many usage groups configured for tenant "tenant1": got %d, max %d`, maxUsageGroups+1, maxUsageGroups),
		},
		{
			Name:     "duplicate_usage_group_name",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name="foo"}`},
				{"app/foo": `{service_name="bar"}`},
			},
			WantErrMsg: `duplicate usage group name "app/foo" for tenant "tenant1"`,
		},
		{
			Name:     "empty_service_name",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"": `{service_name="foo"}`},
			},
			WantErrMsg: `empty service name in usage group for tenant "tenant1"`,
		},
		{
			Name:     "empty_matchers",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": ``},
			},
			WantErrMsg: `no matchers for usage group "app/foo" and tenant "tenant1"`,
		},
		{
			Name:     "invalid_matchers",
			TenantID: "tenant1",
			UsageGroups: []map[string]string{
				{"app/foo": `{service_name?"foo"`},
			},
			Labels:     phlaremodel.Labels{{Name: "service_name", Value: "foo"}},
			WantErrMsg: `failed to parse matchers for usage group "app/foo" and tenant "tenant1": bad matcher format: service_name?"foo"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			overrides := testNewOverrides(t, tt.UsageGroups)

			ug, err := overrides.DistributorUsageGroups(tt.TenantID)
			if tt.WantErrMsg != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErrMsg)
			} else {
				name := ug.GetUsageGroup(tt.Labels)
				require.NoError(t, err)
				require.Equal(t, tt.WantUsageGroup, name)
			}
		})
	}
}

func testNewOverrides(t *testing.T, usageGroups []map[string]string) *Overrides {
	rc := &RuntimeConfigValues{
		TenantLimits: map[string]*Limits{
			"tenant1": {
				DistributorUsageGroups: usageGroups,
			},
		},
	}

	o, err := newOverrides(rc)
	require.NoError(t, err)
	return o
}

func testGenerateUsageGroups(n int, group map[string]string) []map[string]string {
	groups := make([]map[string]string, n)
	for i := range groups {
		groups[i] = group
	}
	return groups
}
