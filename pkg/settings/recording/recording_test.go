package recording

import (
	"testing"

	"github.com/stretchr/testify/require"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_validateGet(t *testing.T) {
	tests := []struct {
		Name    string
		Req     *settingsv1.GetRecordingRuleRequest
		WantErr string
	}{
		{
			Name: "valid",
			Req: &settingsv1.GetRecordingRuleRequest{
				Id: "random",
			},
			WantErr: "",
		},
		{
			Name: "valid_with_formatted_fields",
			Req: &settingsv1.GetRecordingRuleRequest{
				Id: "  random	",
			},
			WantErr: "",
		},
		{
			Name: "empty_id",
			Req: &settingsv1.GetRecordingRuleRequest{
				Id: "",
			},
			WantErr: "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			err := validateGet(tt.Req)
			if tt.WantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_validateUpsert(t *testing.T) {
	tests := []struct {
		Name    string
		Req     *settingsv1.UpsertRecordingRuleRequest
		WantErr string
	}{
		{
			Name: "valid",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:         "abcdef",
				MetricName: "my_metric",
				Matchers: []string{
					`{ label_a = "A" }`,
					`{ label_b =~ "B" }`,
				},
				GroupBy: []string{
					"label_c",
				},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "label_a", Value: "A"},
					{Name: "label_b", Value: "B"},
				},
				Generation: 1,
			},
			WantErr: "",
		},
		{
			Name: "minimal_valid",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "",
				MetricName:     "my_metric",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: "",
		},
		{
			Name: "valid_with_formatted_fields",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "abcdef",
				MetricName:     "  my_metric	",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: "",
		},
		{
			Name: "empty_id",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "",
				MetricName:     "my_metric",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: "",
		},
		{
			Name: "whitespace_only_id",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "  ",
				MetricName:     "my_metric",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `id "  " must match ^[a-zA-Z]+$`,
		},
		{
			Name: "invalid_id",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "?",
				MetricName:     "my_metric",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `id "?" must match ^[a-zA-Z]+$`,
		},
		{
			Name: "empty_metric_name",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName:     "",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: "metric_name is required",
		},
		{
			Name: "invalid_metric_name",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName:     string([]byte{0xC0, 0xAF}), // invalid utf-8
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `metric_name "\xc0\xaf" must be a valid utf-8 string`,
		},
		{
			Name: "invalid_matchers",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "my_metric",
				Matchers: []string{
					"",
				},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `matcher "" is invalid: unknown position: parse error: unexpected end of input`,
		},
		{
			Name: "invalid_group_by",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "my_metric",
				Matchers:   []string{},
				GroupBy: []string{
					"",
				},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `group_by label "" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_external_label",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "my_metric",
				Matchers:   []string{},
				GroupBy:    []string{},
				ExternalLabels: []*typesv1.LabelPair{
					{
						Name:  string([]byte{0xC0, 0xAF}), // invalid utf-8
						Value: string([]byte{0xC0, 0xAF}), // invalid utf-8
					},
				},
			},
			WantErr: "external_labels name \"\\xc0\\xaf\" must be a valid utf-8 string\nexternal_labels value \"\\xc0\\xaf\" must be a valid utf-8 string",
		},
		{
			Name: "invalid_generation",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "abcdef",
				MetricName:     "my_metric",
				Matchers:       []string{},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
				Generation:     -1,
			},
			WantErr: "generation must be positive",
		},
		{
			Name: "multiple_errors",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "",
				Matchers: []string{
					"",
				},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: "metric_name is required\nmatcher \"\" is invalid: unknown position: parse error: unexpected end of input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			err := validateUpsert(tt.Req)
			if tt.WantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_validateDelete(t *testing.T) {
	tests := []struct {
		Name    string
		Req     *settingsv1.DeleteRecordingRuleRequest
		WantErr string
	}{
		{
			Name: "valid",
			Req: &settingsv1.DeleteRecordingRuleRequest{
				Id: "random",
			},
			WantErr: "",
		},
		{
			Name: "valid_with_formatted_fields",
			Req: &settingsv1.DeleteRecordingRuleRequest{
				Id: "  random	",
			},
			WantErr: "",
		},
		{
			Name: "empty_id",
			Req: &settingsv1.DeleteRecordingRuleRequest{
				Id: "",
			},
			WantErr: "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			err := validateDelete(tt.Req)
			if tt.WantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
