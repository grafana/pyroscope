package metrics

import (
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/goccy/go-json"
	prometheusmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

const (
	envVarRecordingRules = "PYROSCOPE_RECORDING_RULES"
	LabelNameMetricName  = "__name__"
)

type StaticRuler struct {
	rules  map[string][]*model.RecordingRule
	logger log.Logger
}

func NewStaticRulerFromEnvVars(logger log.Logger) (Ruler, error) {
	jsonRules := os.Getenv(envVarRecordingRules)

	var rulesByTenant map[string][]*settingsv1.RecordingRule
	if err := json.Unmarshal([]byte(jsonRules), &rulesByTenant); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recording rules: %w", err)
	}

	ruler := &StaticRuler{
		rules:  make(map[string][]*model.RecordingRule, len(rulesByTenant)),
		logger: logger,
	}
	for tenant, rules := range rulesByTenant {
		rs := make([]*model.RecordingRule, 0, len(rules))
		for _, rule := range rules {
			r, err := newRecordingRule(rule)
			if err == nil {
				rs = append(rs, r)
			} else {
				level.Error(logger).Log("msg", "failed to parse recording rule", "rule", rule, "err", err)
			}
		}
		ruler.rules[tenant] = rs
	}
	return ruler, nil
}

func (r StaticRuler) RecordingRules(tenant string) []*model.RecordingRule {
	return r.rules[tenant]
}

func newRecordingRule(rule *settingsv1.RecordingRule) (*model.RecordingRule, error) {
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
		if matcher.Name == model.LabelNameProfileType {
			profileTypePresent = true
			break
		}
	}
	if !profileTypePresent {
		return nil, fmt.Errorf("no __profile_type__ matcher present")
	}

	r := &model.RecordingRule{
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
		Name:  LabelNameMetricName,
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
