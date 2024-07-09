package frontend

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func (f *Frontend) listMetadata(
	ctx context.Context,
	tenants []string,
	startTime, endTime int64,
	query string,
) ([]*metastorev1.BlockMeta, error) {
	_ = level.Info(f.log).Log("msg", "listing metadata", "tenants", tenants, "start", startTime, "end", endTime, "query", query)
	resp, err := f.metastoreclient.ListBlocksForQuery(ctx, &metastorev1.ListBlocksForQueryRequest{
		TenantId:  tenants,
		StartTime: startTime,
		EndTime:   endTime,
		Query:     query,
	})
	if err != nil {
		// TODO: Not sure if we want to pass it through
		return nil, err
	}
	_ = level.Info(f.log).Log("msg", "block metadata list", "blocks", len(resp.Blocks))
	return resp.Blocks, nil
}

func buildQueryFromMatchers(matchers []string) (string, error) {
	parsed, err := parseMatchers(matchers)
	if err != nil {
		return "", fmt.Errorf("parsing label selector: %w", err)
	}
	return matchersToLabelSelector(parsed), nil
}

func buildQueryFromLabelSelectorAndProfileType(labelSelector, profileTypeID string) (string, error) {
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

func findReport[T any](r *T, reports []*querybackendv1.Report) bool {
	for _, x := range reports {
		if reflect.TypeOf(x.ReportType) == reflect.TypeOf(r) {
			if v, ok := (x.ReportType).(any).(*T); ok {
				*r = *v
				return true
			}
		}
	}
	return false
}
