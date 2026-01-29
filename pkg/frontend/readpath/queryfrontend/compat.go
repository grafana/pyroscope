package queryfrontend

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/querybackend"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/util/http"
)

// querySingle is a helper method that expects a single report
// of the appropriate type in the response; this method in an
// adapter to the old query API.
func (q *QueryFrontend) querySingle(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (*queryv1.Report, error) {
	if len(req.Query) != 1 {
		// Nil report is a valid response.
		return nil, nil
	}
	t := querybackend.QueryReportType(req.Query[0].QueryType)
	resp, err := q.Query(ctx, req)
	if err != nil {
		code, sanitized := http.ClientHTTPStatusAndError(err)
		return nil, connect.NewError(connectgrpc.HTTPToCode(int32(code)), sanitized)
	}
	var r *queryv1.Report
	for _, x := range resp.Reports {
		if x.ReportType == t {
			r = x
			break
		}
	}
	return r, nil
}

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
		q.WriteString(m.String())
	}
	q.WriteByte('}')
	return q.String()
}
