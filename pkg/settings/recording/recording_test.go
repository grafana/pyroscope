package recording

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/validation"
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
				MetricName: "profiles_recorded_my_metric",
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
				MetricName:     "profiles_recorded_my_metric",
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
				MetricName:     "  profiles_recorded_my_metric	",
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
				MetricName:     "profiles_recorded_my_metric",
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
				MetricName:     "profiles_recorded_my_metric",
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
				MetricName:     "profiles_recorded_my_metric",
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
			WantErr: "metric_name \"\\xc0\\xaf\" is invalid: invalid metric name: \xc0\xaf",
		},
		{
			Name: "invalid_matchers",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers: []string{
					"",
				},
				GroupBy:        []string{},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `matcher "" is invalid: unknown position: parse error: unexpected end of input`,
		},
		{
			Name: "invalid_group_by_empty",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy: []string{
					"",
				},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `group_by label "" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_group_by_with_dot",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy: []string{
					"service.name",
				},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `group_by label "service.name" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_group_by_with_utf8",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy: []string{
					"世界",
				},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `group_by label "世界" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_group_by_starts_with_number",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy: []string{
					"123invalid",
				},
				ExternalLabels: []*typesv1.LabelPair{},
			},
			WantErr: `group_by label "123invalid" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_external_label_utf8",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy:    []string{},
				ExternalLabels: []*typesv1.LabelPair{
					{
						Name:  string([]byte{0xC0, 0xAF}), // invalid utf-8
						Value: string([]byte{0xC0, 0xAF}), // invalid utf-8
					},
				},
			},
			WantErr: `external_labels name "\xc0\xaf" must match ^[a-zA-Z_][a-zA-Z0-9_]*$
external_labels value "\xc0\xaf" must be a valid utf-8 string`,
		},
		{
			Name: "invalid_external_label_with_dot",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy:    []string{},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "service.name", Value: "foo"},
				},
			},
			WantErr: `external_labels name "service.name" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_external_label_with_utf8_name",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy:    []string{},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "世界", Value: "value"},
				},
			},
			WantErr: `external_labels name "世界" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_external_label_starts_with_number",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				MetricName: "profiles_recorded_my_metric",
				Matchers:   []string{},
				GroupBy:    []string{},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "123invalid", Value: "value"},
				},
			},
			WantErr: `external_labels name "123invalid" must match ^[a-zA-Z_][a-zA-Z0-9_]*$`,
		},
		{
			Name: "invalid_generation",
			Req: &settingsv1.UpsertRecordingRuleRequest{
				Id:             "abcdef",
				MetricName:     "profiles_recorded_my_metric",
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

func Test_idForRule(t *testing.T) {
	tests := []struct {
		name       string
		rule       *settingsv1.RecordingRule
		expectedId string
	}{
		{
			name:       "some-rule",
			expectedId: "veouCnOZTo",
			rule: &settingsv1.RecordingRule{
				MetricName:  "metric1",
				ProfileType: "cpu",
				Matchers: []string{
					`{ label_a = "A" }`,
					`{ label_b =~ "B" }`,
				},
				GroupBy: []string{"label_c", "label_d"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "label_e", Value: "E"},
					{Name: "label_f", Value: "F"},
				},
				StacktraceFilter: &settingsv1.StacktraceFilter{
					FunctionName: &settingsv1.StacktraceFilterFunctionName{
						FunctionName: "function_name",
					},
				},
			},
		},
		{
			name:       "some-other-rule",
			expectedId: "XMMpSpeTom",
			rule: &settingsv1.RecordingRule{
				MetricName:  "metric1",
				ProfileType: "cpu",
				Matchers: []string{
					`{ label_a = "A" }`,
					`{ label_b =~ "B" }`,
				},
				GroupBy: []string{"label_c", "label_d"},
				ExternalLabels: []*typesv1.LabelPair{
					{Name: "label_e", Value: "E"},
					{Name: "label_f", Value: "F"},
				},
				StacktraceFilter: &settingsv1.StacktraceFilter{
					FunctionName: &settingsv1.StacktraceFilterFunctionName{
						FunctionName: "another_function_name",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := idForRule(tt.rule)
			require.Equal(t, tt.expectedId, result)
		})
	}
}

type testRecordingRules struct {
	*RecordingRules
	bucketPath string
}

func newTestRecordingRules(t *testing.T, overrides *validation.Overrides) *testRecordingRules {
	logger := log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	bucketPath := t.TempDir()
	bucket, err := filesystem.NewBucket(bucketPath)
	require.NoError(t, err)

	return &testRecordingRules{
		RecordingRules: New(bucket, logger, overrides),
		bucketPath:     bucketPath,
	}
}

func TestRecordingRules_Get(t *testing.T) {
	testUser := "user1"
	storeRule1 := RandomRule()
	storeRule2 := RandomRule()
	configRule1 := RandomRule()
	configRule1.Id = storeRule2.Id // configRule1 overrides storeRule2
	configRule2 := RandomRule()
	configRule2.Id = "" // this rule doesn't override any rule

	r := newTestRecordingRules(t, validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		user1 := validation.MockDefaultLimits()
		user1.RecordingRules = validation.RecordingRules{
			configRule1,
			configRule2,
		}
		tenantLimits[testUser] = user1
	}))

	ctx := user.InjectOrgID(context.Background(), testUser)

	t.Run("Get not found", func(t *testing.T) {
		_, err := r.GetRecordingRule(ctx, connect.NewRequest(&settingsv1.GetRecordingRuleRequest{Id: storeRule1.Id}))
		require.EqualError(t, err, fmt.Sprintf("not_found: no rule with id='%s' found", storeRule1.Id))
	})

	t.Run("List rules empty for other user", func(t *testing.T) {
		ctx2 := user.InjectOrgID(context.Background(), "user2")
		resp, err := r.ListRecordingRules(ctx2, connect.NewRequest(&settingsv1.ListRecordingRulesRequest{}))
		require.NoError(t, err)
		require.Empty(t, resp.Msg.Rules)
	})

	t.Run("Get config rule from autogenerated ID", func(t *testing.T) {
		idForConfigRule2 := idForRule(configRule2)
		resp, err := r.GetRecordingRule(ctx, connect.NewRequest(&settingsv1.GetRecordingRuleRequest{Id: idForConfigRule2}))
		require.NoError(t, err)
		require.Equal(t, configRule2, resp.Msg.Rule)
	})

	t.Run("Insert store rules", func(t *testing.T) {
		rule, err := r.UpsertRecordingRule(ctx, connect.NewRequest(&settingsv1.UpsertRecordingRuleRequest{
			Id:               storeRule1.Id,
			MetricName:       storeRule1.MetricName,
			Matchers:         storeRule1.Matchers,
			GroupBy:          storeRule1.GroupBy,
			Generation:       storeRule1.Generation,
			ExternalLabels:   storeRule1.ExternalLabels,
			StacktraceFilter: storeRule1.StacktraceFilter,
		}))
		require.NoError(t, err)
		require.Equal(t, storeRule1, rule.Msg.Rule)
		rule, err = r.UpsertRecordingRule(ctx, connect.NewRequest(&settingsv1.UpsertRecordingRuleRequest{
			Id:               storeRule2.Id,
			MetricName:       storeRule2.MetricName,
			Matchers:         storeRule2.Matchers,
			GroupBy:          storeRule2.GroupBy,
			Generation:       storeRule2.Generation,
			ExternalLabels:   storeRule2.ExternalLabels,
			StacktraceFilter: storeRule2.StacktraceFilter,
		}))
		require.NoError(t, err)
		require.Equal(t, storeRule2, rule.Msg.Rule)
	})

	t.Run("Get overridden rule from config", func(t *testing.T) {
		resp, err := r.GetRecordingRule(ctx, connect.NewRequest(&settingsv1.GetRecordingRuleRequest{Id: storeRule2.Id}))
		require.NoError(t, err)
		require.NotEqual(t, storeRule2, configRule1)
		require.Equal(t, configRule1, resp.Msg.Rule)
	})

	t.Run("List rules with overrides", func(t *testing.T) {
		resp, err := r.ListRecordingRules(ctx, connect.NewRequest(&settingsv1.ListRecordingRulesRequest{}))
		require.NoError(t, err)
		require.EqualValues(t, resp.Msg.Rules, []*settingsv1.RecordingRule{
			configRule1,
			configRule2,
			storeRule1,
			// No storeRule2 as it's overridden by configRule1
		})
	})

	t.Run("Upsert overridden rule just changes the original", func(t *testing.T) {
		upsertedRule, err := r.UpsertRecordingRule(ctx, connect.NewRequest(&settingsv1.UpsertRecordingRuleRequest{
			Id:               storeRule2.Id,
			MetricName:       storeRule2.MetricName,
			Matchers:         storeRule2.Matchers,
			GroupBy:          storeRule2.GroupBy,
			Generation:       storeRule2.Generation,
			ExternalLabels:   storeRule2.ExternalLabels,
			StacktraceFilter: storeRule2.StacktraceFilter,
		}))
		storeRule2.Generation++
		require.NoError(t, err)
		require.Equal(t, storeRule2, upsertedRule.Msg.Rule)

		rule, err := r.GetRecordingRule(ctx, connect.NewRequest(&settingsv1.GetRecordingRuleRequest{Id: storeRule2.Id}))
		require.NoError(t, err)
		require.Equal(t, configRule1, rule.Msg.Rule)
	})

	t.Run("Delete store rules", func(t *testing.T) {
		_, err := r.DeleteRecordingRule(ctx, connect.NewRequest(&settingsv1.DeleteRecordingRuleRequest{Id: storeRule1.Id}))
		require.NoError(t, err)
		_, err = r.DeleteRecordingRule(ctx, connect.NewRequest(&settingsv1.DeleteRecordingRuleRequest{Id: storeRule2.Id}))
		require.NoError(t, err)
		resp, err := r.ListRecordingRules(ctx, connect.NewRequest(&settingsv1.ListRecordingRulesRequest{}))
		require.NoError(t, err)
		require.EqualValues(t, resp.Msg.Rules, []*settingsv1.RecordingRule{
			configRule1,
			configRule2,
		})
	})

	t.Run("Can't delete config rules", func(t *testing.T) {
		_, err := r.DeleteRecordingRule(ctx, connect.NewRequest(&settingsv1.DeleteRecordingRuleRequest{Id: configRule1.Id}))

		require.EqualError(t, err, fmt.Sprintf("not_found: no rule with ID='%s' found", configRule1.Id))
	})

}

func init() {
	rand.Seed(uint64(time.Now().UnixNano()))
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func RandomRule() *settingsv1.RecordingRule {
	profileType := RandomString(5)
	matchers := make([]string, rand.Intn(3))
	for i := range matchers {
		matchers[i] = fmt.Sprintf(`{ %s = "%s" }`, RandomString(5), RandomString(5))
	}
	matchers = append(matchers, fmt.Sprintf(`{ __profile_type__ = "%s" }`, profileType))
	groupBy := make([]string, rand.Intn(2)+1)
	for i := range groupBy {
		groupBy[i] = RandomString(5)
	}
	externalLabels := make([]*typesv1.LabelPair, rand.Intn(2)+1)
	for i := range externalLabels {
		externalLabels[i] = &typesv1.LabelPair{
			Name:  RandomString(5),
			Value: RandomString(5),
		}
	}
	var functionFilter *settingsv1.StacktraceFilter
	if rand.Intn(2) == 1 {
		functionFilter = &settingsv1.StacktraceFilter{
			FunctionName: &settingsv1.StacktraceFilterFunctionName{
				FunctionName: RandomString(5),
			},
		}
	}
	return &settingsv1.RecordingRule{
		Id:               RandomString(10),
		MetricName:       "profiles_recorded_" + RandomString(5),
		ProfileType:      profileType,
		Matchers:         matchers,
		GroupBy:          groupBy,
		Generation:       1,
		ExternalLabels:   externalLabels,
		StacktraceFilter: functionFilter,
	}
}
