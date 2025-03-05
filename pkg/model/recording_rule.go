package model

import (
	"fmt"

	prometheusmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type RecordingRule struct {
	Matchers       []*labels.Matcher
	GroupBy        []string
	ExternalLabels labels.Labels
}

func NewRecordingRule(metricName string, matchers []string, groupBy []string, externalLabels labels.Labels) (*RecordingRule, error) {
	// validate metric name
	if !prometheusmodel.IsValidMetricName(prometheusmodel.LabelValue(metricName)) {
		return nil, fmt.Errorf("invalid metric name: %s", metricName)
	}

	// ensure __profile_type__ matcher is present
	ms, err := parseMatchers(matchers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse matchers: %w", err)
	}
	var profileTypePresent bool
	for _, matcher := range ms {
		if matcher.Name == LabelNameProfileType {
			profileTypePresent = true
			break
		}
	}
	if !profileTypePresent {
		return nil, fmt.Errorf("no __profile_type__ matcher present")
	}

	r := &RecordingRule{
		Matchers:       ms,
		GroupBy:        groupBy,
		ExternalLabels: make(labels.Labels, 0, len(externalLabels)+1),
	}

	// ensure __name__ is unique
	for _, lbl := range externalLabels {
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
		Value: metricName,
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
