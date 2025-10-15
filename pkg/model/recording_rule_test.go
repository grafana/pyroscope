package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_NewRecordingRule_GroupByValidation(t *testing.T) {
	tests := []struct {
		name    string
		rule    *settingsv1.RecordingRule
		wantErr string
	}{
		{
			name: "valid group_by",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label", "another_valid"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid group_by with dot",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"service.name"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `group_by label "service.name" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "invalid group_by with UTF-8",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"世界"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `group_by label "世界" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "invalid group_by starts with number",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"123invalid"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `group_by label "123invalid" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "invalid group_by with hyphen",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"service-name"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `group_by label "service-name" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRecordingRule(tt.rule)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_NewRecordingRule_ExternalLabelsValidation(t *testing.T) {
	tests := []struct {
		name    string
		rule    *settingsv1.RecordingRule
		wantErr string
	}{
		{
			name: "valid external_labels",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "valid_label", Value: "value"},
					{Name: "another_valid", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid external_labels with dot",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "service.name", Value: "foo"},
				},
			},
			wantErr: `external_labels name "service.name" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "invalid external_labels with UTF-8",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "世界", Value: "value"},
				},
			},
			wantErr: `external_labels name "世界" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "invalid external_labels starts with number",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "123invalid", Value: "value"},
				},
			},
			wantErr: `external_labels name "123invalid" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "invalid external_labels with hyphen",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "service-name", Value: "value"},
				},
			},
			wantErr: `external_labels name "service-name" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			name: "valid external_labels UTF-8 value is allowed",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "valid_label", Value: "世界"},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRecordingRule(tt.rule)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_NewRecordingRule_MetricNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		rule    *settingsv1.RecordingRule
		wantErr string
	}{
		{
			name: "valid metric_name",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "valid metric_name with underscores and numbers",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test_123",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid metric_name with dot",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded.test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `invalid metric name: profiles_recorded.test`,
		},
		{
			name: "invalid metric_name with UTF-8",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_世界",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `invalid metric name: profiles_recorded_世界`,
		},
		{
			name: "invalid metric_name with emoji",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test_😘",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `invalid metric name: profiles_recorded_test_😘`,
		},
		{
			name: "invalid metric_name starts with number",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "123_invalid",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `invalid metric name: 123_invalid`,
		},
		{
			name: "invalid metric_name with hyphen",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded-test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `invalid metric name: profiles_recorded-test`,
		},
		{
			name: "invalid metric_name with exclamation marks",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test!!!",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: `invalid metric name: profiles_recorded_test!!!`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRecordingRule(tt.rule)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_NewRecordingRule_MatchersValidation(t *testing.T) {
	tests := []struct {
		name    string
		rule    *settingsv1.RecordingRule
		wantErr string
	}{
		{
			name: "valid matchers with standard characters",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{__profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "valid matchers with UTF-8 characters",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{service_name="世界", __profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "valid matchers with UTF-8 label names (quoted)",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{"世界"="value", __profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "valid matchers with emojis in label names and values",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{"utf-8 matchers are fine 🔥"="we don't export them", __profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
		{
			name: "valid matchers with dots in label names",
			rule: &settingsv1.RecordingRule{
				Id:         "test",
				MetricName: "profiles_recorded_test",
				Matchers:   []string{`{"service.name"="my-service", __profile_type__="cpu"}`},
				GroupBy:    []string{"valid_label"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "external_label", Value: "value"},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRecordingRule(tt.rule)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
