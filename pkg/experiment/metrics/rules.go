package metrics

import (
	"fmt"
	"os"
	"regexp"

	"github.com/goccy/go-json"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

const (
	envVarRecordingRules = "RECORDING_RULES"
	LabelNameMetricName  = "__name__"
)

var (
	validMetricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
)

type Ruler interface {
	// RecordingRules return a validated set of rules for a tenant, with the following guarantees:
	// - a "__name__" label is present among ExternalLabels. It contains a valid prometheus metric name.
	// - a matcher with name "__profile__type__" is present in Matchers
	RecordingRules(tenant string) []*RecordingRule
}

type StaticAnonymousRuler struct {
	rules []*RecordingRule
}

type RecordingRule struct {
	Matchers       []*labels.Matcher
	KeepLabels     []string
	ExternalLabels labels.Labels
}

func NewStaticRulerFromEnvVars() (Ruler, error) {
	jsonRules := os.Getenv(envVarRecordingRules)

	var rules []*settingsv1.RecordingRule
	if err := json.Unmarshal([]byte(jsonRules), &rules); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recording rules: %w", err)
	}

	ruler := &StaticAnonymousRuler{
		rules: make([]*RecordingRule, 0, len(rules)),
	}
	for _, rule := range rules {
		r, err := validateRule(rule)
		if err == nil {
			ruler.rules = append(ruler.rules, r)
		}
	}
	return ruler, nil
}

func (r StaticAnonymousRuler) RecordingRules(string) []*RecordingRule {
	return r.rules
}

func validateRule(rule *settingsv1.RecordingRule) (*RecordingRule, error) {
	// validate metric name
	if !validMetricName.MatchString(rule.MetricName) {
		return nil, fmt.Errorf("invalid metric name: %s", rule.MetricName)
	}

	// ensure __profile_type__ matcher is present
	matchers := parseMatchers(rule.Matchers)
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

	r := &RecordingRule{
		Matchers:       parseMatchers(rule.Matchers),
		KeepLabels:     rule.GroupBy,
		ExternalLabels: make(labels.Labels, 0, len(rule.ExternalLabels)+1),
	}

	// ensure __name__ is unique
	for _, lbl := range rule.ExternalLabels {
		if lbl.Name == LabelNameMetricName {
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

func parseMatchers(matchers []string) []*labels.Matcher {
	parsed := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		s, _ := parser.ParseMetricSelector(m)
		parsed = append(parsed, s...)
	}
	return parsed
}
