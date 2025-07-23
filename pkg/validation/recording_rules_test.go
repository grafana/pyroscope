package validation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const recordingRulesOverrideConfig = `
overrides:
  empty-overrides: {}
  empty-rules:
    recording_rules: []
  some-rules:
    recording_rules:
    - metric_name: 'any_name'
      matchers: ['{__profile_type__="any-profile-type", foo="bar"}']
      group_by: ['any-group-by']
      external_labels:
        - name: 'any-label-name'
          value: 'any-label-value'
`

func Test_RecordingRules(t *testing.T) {
	rc, err := LoadRuntimeConfig(bytes.NewReader([]byte(recordingRulesOverrideConfig)))
	require.NoError(t, err)

	o, err := newOverrides(rc)
	require.NoError(t, err)

	rules := o.RecordingRules("no-overrides")
	assert.Equal(t, 0, len(rules))

	rules = o.RecordingRules("empty-overrides")
	assert.Equal(t, 0, len(rules))

	rules = o.RecordingRules("empty-rules")
	assert.Equal(t, 0, len(rules))

	rules = o.RecordingRules("some-rules")
	assert.Equal(t, []*settingsv1.RecordingRule{
		{
			MetricName:     "any_name",
			Matchers:       []string{"{__profile_type__=\"any-profile-type\", foo=\"bar\"}"},
			GroupBy:        []string{"any-group-by"},
			ExternalLabels: []*typesv1.LabelPair{{Name: "any-label-name", Value: "any-label-value"}},
		},
	}, rules)

	_, err = LoadRuntimeConfig(bytes.NewReader([]byte(`
overrides:
  wrong_name:
    recording_rules:
    - metric_name: ""
  `)))
	require.ErrorContains(t, err, "invalid metric name: ")

	_, err = LoadRuntimeConfig(bytes.NewReader([]byte(`
overrides:
  malformed_matchers:
    recording_rules:
    - metric_name: 'any_name'
      matchers: ['{foo="bar}']
  `)))
	require.ErrorContains(t, err, "failed to parse matchers")

	_, err = LoadRuntimeConfig(bytes.NewReader([]byte(`
overrides:
  missing_profile_type:
    recording_rules:
    - metric_name: 'any_name'
      matchers: ['{foo="bar"}']
  `)))
	require.ErrorContains(t, err, "no __profile_type__ matcher present")
}
