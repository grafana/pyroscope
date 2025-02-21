package model

import (
	"fmt"

	prometheusmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

type RecordingRule struct {
	Matchers       []*labels.Matcher
	GroupBy        []string
	ExternalLabels labels.Labels
}

func NewRecordingRule(rule *settingsv1.RecordingRule) (*RecordingRule, error) {
	// validate metric name
	if !prometheusmodel.IsValidMetricName(prometheusmodel.LabelValue(rule.MetricName)) {
		return nil, fmt.Errorf("invalid metric name: %s", rule.MetricName)
	}

	// ensure __profile_type__ matcher is present
	matchers, err := parseMatchers(rule.Matchers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse matchers: %w", err)
	}
	var profileTypePresent bool
	for _, matcher := range matchers {
		if matcher.Name == LabelNameProfileType {
			profileTypePresent = true
			break
		}
	}
	if !profileTypePresent {
		return nil, fmt.Errorf("no __profile_type__ matcher present")
	}

	r := &RecordingRule{
		Matchers:       matchers,
		GroupBy:        rule.GroupBy,
		ExternalLabels: make(labels.Labels, 0, len(rule.ExternalLabels)+1),
	}

	// ensure __name__ is unique
	for _, lbl := range rule.ExternalLabels {
		if lbl.Name == prometheusmodel.MetricNameLabel {
			// skip __name__
			continue
		}
		r.ExternalLabels = append(r.ExternalLabels, labels.Label{
			Name:  lbl.Name,
			Value: lbl.Value,
		})
	}
	// trust rule.MetricName
	r.ExternalLabels = append(r.ExternalLabels, labels.Label{
		Name:  prometheusmodel.MetricNameLabel,
		Value: rule.MetricName,
	})
	return r, nil
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
