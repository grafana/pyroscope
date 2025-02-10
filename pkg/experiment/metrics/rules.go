package metrics

import (
	"github.com/prometheus/prometheus/model/labels"
)

type RecordingRule struct {
	profileType string
	metricName  string
	matchers    []*labels.Matcher
	keepLabels  []string
}

func recordingRulesFromTenant(tenant string) []*RecordingRule {
	// TODO
	return []*RecordingRule{}
}
