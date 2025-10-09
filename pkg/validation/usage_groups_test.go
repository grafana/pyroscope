package validation

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestUsageGroupConfig_GetUsageGroups(t *testing.T) {
	tests := []struct {
		Name        string
		TenantID    string
		Config      map[string]string
		Labels      phlaremodel.Labels
		WantedNames []string
	}{
		{
			Name:     "single_usage_group_match",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/foo": `{service_name="foo"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
			},
			WantedNames: []string{"app/foo"},
		},
		{
			Name:     "multiple_usage_group_matches",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/foo":  `{service_name="foo"}`,
				"app/foo2": `{service_name="foo", namespace=~"bar.*"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
				{Name: "namespace", Value: "barbaz"},
			},
			WantedNames: []string{
				"app/foo",
				"app/foo2",
			},
		},
		{
			Name:     "no_usage_group_matches",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/foo": `{service_name="notfound"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
			},
			WantedNames: []string{},
		},
		{
			Name:     "wildcard_matcher",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/foo": `{}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
			},
			WantedNames: []string{"app/foo"},
		},
		{
			Name:     "no_labels",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/foo": `{service_name="foo"}`,
			},
			Labels:      phlaremodel.Labels{},
			WantedNames: []string{},
		},
		{
			Name:     "disjoint_labels_do_not_match",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/foo": `{namespace="foo", container="bar"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
			},
			WantedNames: []string{},
		},
		{
			Name:     "dynamic_usage_group_names",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/${labels.service_name}": `{service_name=~"(.*)"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
			},
			WantedNames: []string{
				"app/foo",
			},
		},
		{
			Name:     "dynamic_usage_group_names_missing_label",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/${labels.service_name}/${labels.env}": `{service_name=~"(.*)"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: "foo"},
			},
			WantedNames: []string{},
		},
		{
			Name:     "dynamic_usage_group_names_empty_label",
			TenantID: "tenant1",
			Config: map[string]string{
				"app/${labels.service_name}": `{service_name=~"(.*)"}`,
			},
			Labels: phlaremodel.Labels{
				{Name: "service_name", Value: ""},
			},
			WantedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			config, err := NewUsageGroupConfig(tt.Config)
			require.NoError(t, err)

			evaluator := NewUsageGroupEvaluator(util.Logger)
			got := evaluator.GetMatch(tt.TenantID, config, tt.Labels)

			gotNames := make([]string, len(got.names))
			for i, name := range got.names {
				gotNames[i] = name.ResolvedName
			}
			slices.Sort(gotNames)
			slices.Sort(tt.WantedNames)
			require.Equal(t, tt.WantedNames, gotNames)
		})
	}
}

func TestUsageGroupMatch_CountReceivedBytes(t *testing.T) {
	tests := []struct {
		Name       string
		Match      UsageGroupMatch
		Count      int64
		WantCounts map[string]float64
	}{
		{
			Name: "single_usage_group_match",
			Match: UsageGroupMatch{
				tenantID: "tenant1",
				names:    []UsageGroupMatchName{{ResolvedName: "app/foo"}},
			},
			Count: 100,
			WantCounts: map[string]float64{
				"app/foo":  100,
				"app/foo2": 0,
				"other":    0,
			},
		},
		{
			Name: "multiple_usage_group_matches",
			Match: UsageGroupMatch{
				tenantID: "tenant1",
				names: []UsageGroupMatchName{
					{ResolvedName: "app/foo"},
					{ResolvedName: "app/foo2"},
				},
			},
			Count: 100,
			WantCounts: map[string]float64{
				"app/foo":  100,
				"app/foo2": 100,
				"other":    0,
			},
		},
		{
			Name: "no_usage_group_matches",
			Match: UsageGroupMatch{
				tenantID: "tenant1",
				names:    []UsageGroupMatchName{},
			},
			Count: 100,
			WantCounts: map[string]float64{
				"app/foo":  0,
				"app/foo2": 0,
				"other":    100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			const profileType = "cpu"
			usageGroupReceivedDecompressedBytes.Reset()

			tt.Match.CountReceivedBytes(profileType, tt.Count)

			for name, want := range tt.WantCounts {
				collector := usageGroupReceivedDecompressedBytes.WithLabelValues(
					profileType,
					tt.Match.tenantID,
					name,
				)

				got := testutil.ToFloat64(collector)
				require.Equal(t, got, want, "usage group %s has incorrect metric value", name)
			}
		})
	}
}

func TestUsageGroupMatch_CountDiscardedBytes(t *testing.T) {
	tests := []struct {
		Name       string
		Match      UsageGroupMatch
		Count      int64
		WantCounts map[string]float64
	}{
		{
			Name: "single_usage_group_match",
			Match: UsageGroupMatch{
				tenantID: "tenant1",
				names:    []UsageGroupMatchName{{ResolvedName: "app/foo"}},
			},
			Count: 100,
			WantCounts: map[string]float64{
				"app/foo":  100,
				"app/foo2": 0,
				"other":    0,
			},
		},
		{
			Name: "multiple_usage_group_matches",
			Match: UsageGroupMatch{
				tenantID: "tenant1",
				names: []UsageGroupMatchName{
					{ResolvedName: "app/foo"},
					{ResolvedName: "app/foo2"},
				},
			},
			Count: 100,
			WantCounts: map[string]float64{
				"app/foo":  100,
				"app/foo2": 100,
				"other":    0,
			},
		},
		{
			Name: "no_usage_group_matches",
			Match: UsageGroupMatch{
				tenantID: "tenant1",
				names:    []UsageGroupMatchName{},
			},
			Count: 100,
			WantCounts: map[string]float64{
				"app/foo":  0,
				"app/foo2": 0,
				"other":    100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			const reason = "no_reason"
			usageGroupDiscardedBytes.Reset()

			tt.Match.CountDiscardedBytes(reason, tt.Count)

			for name, want := range tt.WantCounts {
				collector := usageGroupDiscardedBytes.WithLabelValues(
					reason,
					tt.Match.tenantID,
					name,
				)

				got := testutil.ToFloat64(collector)
				require.Equal(t, got, want, "usage group %q has incorrect metric value", name)
			}
		})
	}
}

func (c *UsageGroupConfig) valuesMap() map[string][]string {
	m := make(map[string][]string)
	for k, v := range c.config {
		for _, matcher := range v {
			m[k] = append(m[k], matcher.String())
		}
	}
	return m
}

func TestNewUsageGroupConfig(t *testing.T) {
	tests := []struct {
		Name      string
		ConfigMap map[string]string
		Want      *UsageGroupConfig
		WantErr   string
	}{
		{
			Name: "single_usage_group",
			ConfigMap: map[string]string{
				"app/foo": `{service_name="foo"}`,
			},
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo": testMustParseMatcher(t, `{service_name="foo"}`),
				},
			},
		},
		{
			Name: "multiple_usage_groups",
			ConfigMap: map[string]string{
				"app/foo":  `{service_name="foo"}`,
				"app/foo2": `{service_name="foo", namespace=~"bar.*"}`,
			},
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo":  testMustParseMatcher(t, `{service_name="foo"}`),
					"app/foo2": testMustParseMatcher(t, `{service_name="foo", namespace=~"bar.*"}`),
				},
			},
		},
		{
			Name:      "no_usage_groups",
			ConfigMap: map[string]string{},
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{},
			},
		},
		{
			Name: "wildcard_matcher",
			ConfigMap: map[string]string{
				"app/foo": `{}`,
			},
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo": testMustParseMatcher(t, `{}`),
				},
			},
		},
		{
			Name: "too_many_usage_groups",
			ConfigMap: func() map[string]string {
				m := make(map[string]string)
				for i := 0; i < maxUsageGroups+1; i++ {
					m[fmt.Sprintf("app/foo%d", i)] = `{service_name="foo"}`
				}
				return m
			}(),
			WantErr: fmt.Sprintf("maximum number of usage groups is %d, got %d", maxUsageGroups, maxUsageGroups+1),
		},
		{
			Name: "invalid_matcher",
			ConfigMap: map[string]string{
				"app/foo": `????`,
			},
			WantErr: `failed to parse matchers for usage group "app/foo": 1:1: parse error: unexpected character: '?'`,
		},
		{
			Name: "empty_matcher",
			ConfigMap: map[string]string{
				"app/foo": ``,
			},
			WantErr: `failed to parse matchers for usage group "app/foo": unknown position: parse error: unexpected end of input`,
		},
		{
			Name: "empty_name",
			ConfigMap: map[string]string{
				"": `{service_name="foo"}`,
			},
			WantErr: "usage group name cannot be empty",
		},
		{
			Name: "whitespace_name",
			ConfigMap: map[string]string{
				"   app/foo   ": `{service_name="foo"}`,
			},
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo": testMustParseMatcher(t, `{service_name="foo"}`),
				},
			},
		},
		{
			Name: "reserved_name",
			ConfigMap: map[string]string{
				noMatchName: `{service_name="foo"}`,
			},
			WantErr: fmt.Sprintf("usage group name %q is reserved", noMatchName),
		},
		{
			Name: "invalid_utf8_name",
			ConfigMap: map[string]string{
				"app/\x80foo": `{service_name="foo"}`,
			},
			WantErr: `usage group name "app/\x80foo" is not valid UTF-8`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := NewUsageGroupConfig(tt.ConfigMap)
			if tt.WantErr != "" {
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want.valuesMap(), got.valuesMap())
			}
		})
	}
}

func TestUsageGroupConfig_UnmarshalYAML(t *testing.T) {
	type Object struct {
		UsageGroups UsageGroupConfig `yaml:"usage_groups"`
	}

	tests := []struct {
		Name    string
		YAML    string
		Want    *UsageGroupConfig
		WantErr string
	}{
		{
			Name: "single_usage_group",
			YAML: `
usage_groups:
  app/foo: '{service_name="foo"}'`,
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo": testMustParseMatcher(t, `{service_name="foo"}`),
				},
			},
		},
		{
			Name: "multiple_usage_groups",
			YAML: `
usage_groups:
  app/foo: '{service_name="foo"}'
  app/foo2: '{service_name="foo", namespace=~"bar.*"}'`,
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo":  testMustParseMatcher(t, `{service_name="foo"}`),
					"app/foo2": testMustParseMatcher(t, `{service_name="foo", namespace=~"bar.*"}`),
				},
			},
		},
		{
			Name: "empty_usage_groups",
			YAML: `
usage_groups: {}`,
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{},
			},
		},
		{
			Name:    "invalid_yaml",
			YAML:    `usage_groups: ?????`,
			WantErr: "malformed usage group config: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `?????` into map[string]string",
		},
		{
			Name: "invalid_matcher",
			YAML: `
usage_groups:
  app/foo: ?????`,
			WantErr: `failed to parse matchers for usage group "app/foo": 1:1: parse error: unexpected character: '?'`,
		},
		{
			Name: "missing_usage_groups_key_in_config",
			YAML: `
some_other_config:
  foo: bar`,
			Want: &UsageGroupConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := Object{}
			err := yaml.Unmarshal([]byte(tt.YAML), &got)
			if tt.WantErr != "" {
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want.valuesMap(), got.UsageGroups.valuesMap())
			}
		})
	}
}

func TestUsageGroupConfig_UnmarshalJSON(t *testing.T) {
	type Object struct {
		UsageGroups UsageGroupConfig `json:"usage_groups"`
	}

	tests := []struct {
		Name    string
		JSON    string
		Want    *UsageGroupConfig
		WantErr string
	}{
		{
			Name: "single_usage_group",
			JSON: `{
				"usage_groups": {
					"app/foo": "{service_name=\"foo\"}"
				}
			}`,
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo": testMustParseMatcher(t, `{service_name="foo"}`),
				},
			},
		},
		{
			Name: "multiple_usage_groups",
			JSON: `{
				"usage_groups": {
					"app/foo": "{service_name=\"foo\"}",
					"app/foo2": "{service_name=\"foo\", namespace=~\"bar.*\"}"
				}
			}`,
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{
					"app/foo":  testMustParseMatcher(t, `{service_name="foo"}`),
					"app/foo2": testMustParseMatcher(t, `{service_name="foo", namespace=~"bar.*"}`),
				},
			},
		},
		{
			Name: "empty_usage_groups",
			JSON: `{"usage_groups": {}}`,
			Want: &UsageGroupConfig{
				config: map[string][]*labels.Matcher{},
			},
		},
		{
			Name:    "invalid_json",
			JSON:    `{"usage_groups": "?????"}`,
			WantErr: "malformed usage group config: json: cannot unmarshal string into Go value of type map[string]string",
		},
		{
			Name:    "invalid_matcher",
			JSON:    `{"usage_groups": {"app/foo": "?????"}}`,
			WantErr: `failed to parse matchers for usage group "app/foo": 1:1: parse error: unexpected character: '?'`,
		},
		{
			Name: "missing_usage_groups_key_in_config",
			JSON: `{"some_other_key": {"foo": "bar"}}`,
			Want: &UsageGroupConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := Object{}
			err := json.Unmarshal([]byte(tt.JSON), &got)
			if tt.WantErr != "" {
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want.valuesMap(), got.UsageGroups.valuesMap())
			}
		})
	}
}

func testMustParseMatcher(t *testing.T, s string) []*labels.Matcher {
	m, err := parser.ParseMetricSelector(s)
	require.NoError(t, err)
	return m
}

func TestUsageGroupMatchName_IsMoreSpecificThan(t *testing.T) {
	tests := []struct {
		Name  string
		Match UsageGroupMatchName
		Other UsageGroupMatchName
		Want  bool
	}{
		{
			Name:  "same name",
			Match: UsageGroupMatchName{ConfiguredName: "app/foo"},
			Other: UsageGroupMatchName{ConfiguredName: "app/foo"},
			Want:  false,
		},
		{
			Name:  "less specific name",
			Match: UsageGroupMatchName{ConfiguredName: "${labels.service_name}"},
			Other: UsageGroupMatchName{ConfiguredName: "test-service"},
			Want:  false,
		},
		{
			Name:  "more specific name",
			Match: UsageGroupMatchName{ConfiguredName: "test-service"},
			Other: UsageGroupMatchName{ConfiguredName: "${labels.service_name}"},
			Want:  true,
		},
		{
			Name:  "more specific name with prefix",
			Match: UsageGroupMatchName{ConfiguredName: "test-service"},
			Other: UsageGroupMatchName{ConfiguredName: "service/${labels.service_name}"},
			Want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			require.Equal(t, tt.Want, tt.Match.IsMoreSpecificThan(&tt.Other))
		})
	}
}
