package model

import (
	"fmt"
	"strings"

	prometheusmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

type RecordingRule struct {
	Matchers       []*labels.Matcher
	GroupBy        []string
	ExternalLabels labels.Labels
	FunctionName   string
}

const (
	metricNamePrefix = "profiles_recorded_"
	RuleIDLabel      = "profiles_rule_id"
)

var uniqueLabels = map[string]bool{
	RuleIDLabel:                     true,
	prometheusmodel.MetricNameLabel: true,
}

func NewRecordingRule(rule *settingsv1.RecordingRule) (*RecordingRule, error) {
	sb := labels.NewScratchBuilder(len(rule.ExternalLabels) + 1)
	return newRecordingRuleWithBuilder(rule, &sb)
}

func newRecordingRuleWithBuilder(rule *settingsv1.RecordingRule, sb *labels.ScratchBuilder) (*RecordingRule, error) {
	// validate metric name
	if err := ValidateMetricName(rule.MetricName); err != nil {
		return nil, err
	}

	// ensure __profile_type__ matcher is present
	matchers, err := parseMatchers(rule.Matchers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse matchers: %w", err)
	}
	var profileTypeMatcher *labels.Matcher
	for _, matcher := range matchers {
		if matcher.Name == LabelNameProfileType {
			profileTypeMatcher = matcher
			break
		}
	}
	if profileTypeMatcher == nil {
		return nil, fmt.Errorf("no __profile_type__ matcher present")
	}
	if profileTypeMatcher.Type != labels.MatchEqual {
		return nil, fmt.Errorf("__profile_type__ matcher is not an equality")
	}
	var functionName string
	if rule.StacktraceFilter != nil {
		if rule.StacktraceFilter.FunctionName != nil {
			functionName = rule.StacktraceFilter.FunctionName.FunctionName
		}
	}

	// validate group_by label names for Prometheus compatibility
	for _, labelName := range rule.GroupBy {
		name := prometheusmodel.LabelName(labelName)
		if !prometheusmodel.LegacyValidation.IsValidLabelName(string(name)) {
			return nil, fmt.Errorf("group_by label %q must match %s", labelName, prometheusmodel.LabelNameRE.String())
		}
	}

	sb.Reset()
	for _, lbl := range rule.ExternalLabels {
		// ensure no __name__ or profiles_rule_id labels already exist
		if uniqueLabels[lbl.Name] {
			// skip
			continue
		}
		// validate external label names for Prometheus compatibility
		name := prometheusmodel.LabelName(lbl.Name)
		if !prometheusmodel.LegacyValidation.IsValidLabelName(string(name)) {
			return nil, fmt.Errorf("external_labels name %q must match %s", lbl.Name, prometheusmodel.LabelNameRE.String())
		}
		sb.Add(lbl.Name, lbl.Value)
	}

	// trust rule.MetricName
	sb.Add(prometheusmodel.MetricNameLabel, rule.MetricName)
	// Inject recording rule Id
	sb.Add(RuleIDLabel, rule.Id)

	sb.Sort()

	return &RecordingRule{
		Matchers:       matchers,
		GroupBy:        rule.GroupBy,
		ExternalLabels: sb.Labels(),
		FunctionName:   functionName,
	}, nil
}

func parseMatchers(matchers []string) ([]*labels.Matcher, error) {
	parsed := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		s, err := parser.ParseMetricSelector(m)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, s...)
	}
	return parsed, nil
}

func ValidateMetricName(name string) error {
	if !prometheusmodel.LegacyValidation.IsValidMetricName(name) {
		return fmt.Errorf("invalid metric name: %s", name)
	}
	if !strings.HasPrefix(name, metricNamePrefix) {
		return fmt.Errorf("metric name must start with %s", metricNamePrefix)
	}
	return nil
}
