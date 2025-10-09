package metrics

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/validation"
)

var (
	defaultRecordingRulesProto = []*settingsv1.RecordingRule{{
		Id:         "some-id",
		MetricName: "profiles_recorded_default_recording_rule",
		Matchers:   []string{"{__profile_type__=\"any-profile-type\"}"},
	}}

	defaultRecordingRules = []*model.RecordingRule{{
		ExternalLabels: labels.New(labels.Label{
			Name:  "__name__",
			Value: "profiles_recorded_default_recording_rule",
		}, labels.Label{
			Name:  "profiles_rule_id",
			Value: "some-id",
		}),
		Matchers: []*labels.Matcher{{
			Type:  labels.MatchEqual,
			Name:  "__profile_type__",
			Value: "any-profile-type",
		}},
	}}

	overriddenRecordingRulesProto = []*settingsv1.RecordingRule{{
		Id:             "another-id",
		MetricName:     "profiles_recorded_rule",
		Matchers:       []string{"{__profile_type__=\"any-profile-type\", matcher1!=\"value\"}"},
		GroupBy:        []string{"group_by_label"},
		ExternalLabels: []*typesv1.LabelPair{{Name: "foo", Value: "bar"}},
	}}

	overriddenRecordingRules = []*model.RecordingRule{{
		Matchers: []*labels.Matcher{
			{Type: labels.MatchEqual, Name: "__profile_type__", Value: "any-profile-type"},
			{Type: labels.MatchNotEqual, Name: "matcher1", Value: "value"},
		},
		GroupBy: []string{"group_by_label"},
		ExternalLabels: labels.New(
			labels.Label{Name: "__name__", Value: "profiles_recorded_rule"},
			labels.Label{Name: "foo", Value: "bar"},
			labels.Label{Name: "profiles_rule_id", Value: "another-id"},
		),
	}}
)

func Test_Ruler_happyPath(t *testing.T) {
	overrides := newOverrides(t)

	ruler := NewStaticRulerFromOverrides(overrides)

	rules := ruler.RecordingRules("non-configured-tenant")
	assert.Equal(t, defaultRecordingRules, rules)

	rules = ruler.RecordingRules("tenant-override")
	assert.Equal(t, overriddenRecordingRules, rules)
}

func newOverrides(t *testing.T) *validation.Overrides {
	t.Helper()
	return validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		defaults.RecordingRules = defaultRecordingRulesProto
		l := validation.MockDefaultLimits()
		l.RecordingRules = overriddenRecordingRulesProto
		tenantLimits["tenant-override"] = l
	})
}
