package model

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_NewRecordingRule_LegacyMetricName(t *testing.T) {
	rule, err := NewRecordingRule(&settingsv1.RecordingRule{
		MetricName:     "profiles_recorded_metric.name.should.not.include.UTF-8.characters!!! nor emojis 😘",
		Matchers:       []string{"{any_matcher=\"any-value\", __profile_type__=\"any-profile-type\"}"},
		GroupBy:        []string{"any_label"},
		ExternalLabels: []*typesv1.LabelPair{{Name: "any_label_name", Value: "any_label_value"}},
	})

	require.ErrorContains(t, err, "invalid metric name")
	require.Nil(t, rule)
}

func Test_NewRecordingRule_LegacyGroupByName(t *testing.T) {
	rule, err := NewRecordingRule(&settingsv1.RecordingRule{
		MetricName:     "profiles_recorded_valid_metric_name",
		Matchers:       []string{"{any_matcher=\"any-value\", __profile_type__=\"any-profile-type\"}"},
		GroupBy:        []string{"group by are exported as labels and.should.not.include.UTF-8.characters!!! nor emojis 🥵"},
		ExternalLabels: []*typesv1.LabelPair{{Name: "any_label_name", Value: "any_label_value"}},
	})

	require.Error(t, err, "we expect a validation error here, group by are label names and should not include UTF-8 characters")
	require.Nil(t, rule)
}
func Test_NewRecordingRule_LegacyLabelName(t *testing.T) {
	rule, err := NewRecordingRule(&settingsv1.RecordingRule{
		MetricName:     "profiles_recorded_valid_metric_name",
		Matchers:       []string{"{any_matcher=\"any-value\", __profile_type__=\"any-profile-type\"}"},
		GroupBy:        []string{"any_label"},
		ExternalLabels: []*typesv1.LabelPair{{Name: "🙏_don't", Value: "any_label_value"}},
	})

	require.Error(t, err, "we expect a validation error here, external label names should not include UTF-8 characters")
	require.Nil(t, rule)
}

func Test_NewRecordingRule_Utf8_matchers(t *testing.T) {
	rule, err := NewRecordingRule(&settingsv1.RecordingRule{
		MetricName:     "profiles_recorded_valid_metric_name",
		Matchers:       []string{"{'utf-8 matchers are fine 🔥'=\"we don't export them\", __profile_type__=\"any-profile-type\"}"},
		GroupBy:        []string{"any_label"},
		ExternalLabels: []*typesv1.LabelPair{{Name: "any_label_name", Value: "any value permitted here 🙌"}},
	})

	require.NoError(t, err)
	require.NotNil(t, rule)
	require.Equal(t, rule.Matchers, []*labels.Matcher{
		{Type: labels.MatchEqual, Name: "utf-8 matchers are fine 🔥", Value: "we don't export them"},
		{Type: labels.MatchEqual, Name: "__profile_type__", Value: "any-profile-type"},
	})
}
