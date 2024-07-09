package validation

import (
	"bytes"
	"flag"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type wrappedRuntimeConfig struct {
	rc *RuntimeConfigValues
}

func (w *wrappedRuntimeConfig) TenantLimits(tenantID string) *Limits {
	return w.rc.TenantLimits[tenantID]
}

func (w *wrappedRuntimeConfig) AllByTenantID() map[string]*Limits {
	return w.rc.TenantLimits
}

func newOverrides(rc *RuntimeConfigValues) (*Overrides, error) {
	var defaultCfg Limits
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	defaultCfg.RegisterFlags(fs)
	return NewOverrides(defaultCfg, &wrappedRuntimeConfig{rc})
}

const tenantOverrideConfig = `
overrides:
  nothing: {}
  disabled:
    ingestion_relabeling_default_rules_position: disabled
  custom-rule-end:
    ingestion_relabeling_rules:
      - action: drop
  custom-rule-start:
    ingestion_relabeling_default_rules_position: last
    ingestion_relabeling_rules:
      - action: drop
  custom-rule-only:
    ingestion_relabeling_default_rules_position: disabled
    ingestion_relabeling_rules:
      - action: drop
  
`

func Test_IngestionRelabelingRules(t *testing.T) {
	rc, err := LoadRuntimeConfig(bytes.NewReader([]byte(tenantOverrideConfig)))
	require.NoError(t, err)

	o, err := newOverrides(rc)
	require.NoError(t, err)

	rules := o.IngestionRelabelingRules("xxxx")
	require.Equal(t, len(defaultRelabelRules), len(rules))

	rules = o.IngestionRelabelingRules("nothing")
	require.Equal(t, len(defaultRelabelRules), len(rules))

	rules = o.IngestionRelabelingRules("disabled")
	require.Equal(t, 0, len(rules))

	rules = o.IngestionRelabelingRules("custom-rule-end")
	require.Equal(t, len(defaultRelabelRules)+1, len(rules))
	require.Equal(t, relabel.Drop, rules[len(defaultRelabelRules)].Action)

	rules = o.IngestionRelabelingRules("custom-rule-start")
	require.Equal(t, len(defaultRelabelRules)+1, len(rules))
	require.Equal(t, relabel.Drop, rules[0].Action)

	rules = o.IngestionRelabelingRules("custom-rule-only")
	require.Equal(t, 1, len(rules))
	require.Equal(t, relabel.Drop, rules[0].Action)

	_, err = LoadRuntimeConfig(bytes.NewReader([]byte(`
overrides:
  wrong-mode:
    ingestion_relabeling_default_rules_position: end
  `)))
	require.ErrorContains(t, err, "invalid ingestion_relabeling_default_rules_position: end")

	_, err = LoadRuntimeConfig(bytes.NewReader([]byte(`
overrides:
  wrong-rule-action:
    ingestion_relabeling_rules: [{action: refund}]
  `)))
	require.ErrorContains(t, err, "unknown relabel action \"refund\"")

	_, err = LoadRuntimeConfig(bytes.NewReader([]byte(`
overrides:
  empty-rule:
    ingestion_relabeling_rules: [{}]
  `)))
	require.ErrorContains(t, err, "relabel configuration for replace action requires 'target_label'")

}

func Test_defaultRelabelRules(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    labels.Labels
		expected labels.Labels
		kept     bool
	}{
		{
			name:     "let empty through",
			input:    labels.Labels{},
			expected: labels.Labels{},
			kept:     true,
		},
		{
			name: "godelta prof remove prefix",
			input: labels.FromStrings(
				phlaremodel.LabelNameProfileName, "godeltaprof_memory", // TODO: Verify is this is really the prefix used
			),
			expected: labels.FromStrings(
				phlaremodel.LabelNameProfileName, "memory",
				"__name_replaced__", "godeltaprof_memory",
				"__delta__", "false",
			),
			kept: true,
		},
		{
			name: "replace wall name with cpu type",
			input: labels.FromStrings(
				phlaremodel.LabelNameProfileName, "wall",
				phlaremodel.LabelNameType, "cpu",
			),
			expected: labels.FromStrings(
				phlaremodel.LabelNameProfileName, "process_cpu",
				"__name_replaced__", "wall",
				phlaremodel.LabelNameType, "cpu",
			),
			kept: true,
		},
	} {
		result, kept := relabel.Process(tc.input, defaultRelabelRules...)
		require.Equal(t, tc.expected, result)
		require.Equal(t, tc.kept, kept)
	}

}
