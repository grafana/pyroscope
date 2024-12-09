package query_frontend

import (
	"fmt"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func buildLabelSelectorFromMatchers(matchers []string) (string, error) {
	parsed, err := parseMatchers(matchers)
	if err != nil {
		return "", fmt.Errorf("parsing label selector: %w", err)
	}
	return matchersToLabelSelector(parsed), nil
}

func buildLabelSelectorWithProfileType(labelSelector, profileTypeID string) (string, error) {
	matchers, err := parser.ParseMetricSelector(labelSelector)
	if err != nil {
		return "", fmt.Errorf("parsing label selector %q: %w", labelSelector, err)
	}
	profileType, err := phlaremodel.ParseProfileTypeSelector(profileTypeID)
	if err != nil {
		return "", fmt.Errorf("parsing profile type ID %q: %w", profileTypeID, err)
	}
	matchers = append(matchers, phlaremodel.SelectorFromProfileType(profileType))
	return matchersToLabelSelector(matchers), nil
}

func parseMatchers(matchers []string) ([]*labels.Matcher, error) {
	parsed := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		s, err := parser.ParseMetricSelector(m)
		if err != nil {
			return nil, fmt.Errorf("failed to parse label selector %q: %w", s, err)
		}
		parsed = append(parsed, s...)
	}
	return parsed, nil
}

func matchersToLabelSelector(matchers []*labels.Matcher) string {
	var q strings.Builder
	q.WriteByte('{')
	for i, m := range matchers {
		if i > 0 {
			q.WriteByte(',')
		}
		q.WriteString(m.Name)
		q.WriteString(m.Type.String())
		q.WriteByte('"')
		q.WriteString(m.Value)
		q.WriteByte('"')
	}
	q.WriteByte('}')
	return q.String()
}

func findReport(r queryv1.ReportType, reports []*queryv1.Report) *queryv1.Report {
	for _, x := range reports {
		if x.ReportType == r {
			return x
		}
	}
	return nil
}
