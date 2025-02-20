package model

import "github.com/prometheus/prometheus/model/labels"

type RecordingRule struct {
	Matchers       []*labels.Matcher
	GroupBy        []string
	ExternalLabels labels.Labels
}
