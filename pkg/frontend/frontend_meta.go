package frontend

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/querybackend"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
)

func (f *Frontend) listMetadata(
	ctx context.Context,
	tenants []string,
	startTime, endTime int64,
	query string,
) ([]*metastorev1.BlockMeta, error) {
	_ = level.Info(f.log).Log("msg", "listing metadata",
		"tenants", strings.Join(tenants, ","),
		"start", startTime,
		"end", endTime,
		"query", query,
	)
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
	printStats(f.log, resp.Blocks)
	return resp.Blocks, nil
}

func printStats(logger log.Logger, blocks []*metastorev1.BlockMeta) {
	type blockMetaStats struct {
		level   uint32
		minTime int64
		maxTime int64
		size    uint64
		count   int
	}
	m := make(map[uint32]*blockMetaStats)
	for _, b := range blocks {
		s, ok := m[b.CompactionLevel]
		if !ok {
			s = &blockMetaStats{level: b.CompactionLevel}
			m[b.CompactionLevel] = s
		}
		for _, x := range b.TenantServices {
			s.size += x.Size
		}
		s.count++
	}
	sorted := make([]*blockMetaStats, 0, len(m))
	for _, s := range m {
		sorted = append(sorted, s)
	}
	slices.SortFunc(sorted, func(a, b *blockMetaStats) int {
		return int(a.level - b.level)
	})
	fields := make([]interface{}, 0, 4+len(sorted)*2)
	fields = append(fields, "msg", "block metadata list", "blocks_total", fmt.Sprint(len(blocks)))
	for _, s := range sorted {
		fields = append(fields,
			fmt.Sprintf("l%d_blocks", s.level), fmt.Sprint(s.count),
			fmt.Sprintf("l%d_size", s.level), fmt.Sprint(s.size),
		)
	}
	_ = level.Info(logger).Log(fields...)
}

func (f *Frontend) invoke(
	ctx context.Context,
	startTime, endTime int64,
	tenants []string,
	labelSelector string,
	q *querybackendv1.Query,
) (*querybackendv1.Report, error) {
	blocks, err := f.listMetadata(ctx, tenants, startTime, endTime, labelSelector)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	// TODO: Params.
	p := queryplan.Build(blocks, 2, 10)
	resp, err := f.querybackendclient.Invoke(ctx, &querybackendv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     startTime,
		EndTime:       endTime,
		LabelSelector: labelSelector,
		Options:       &querybackendv1.InvokeOptions{},
		QueryPlan:     p.Proto(),
		Query:         []*querybackendv1.Query{q},
	})
	if err != nil {
		return nil, err
	}
	return findReport(querybackend.QueryReportType(q.QueryType), resp.Reports), nil
}

func buildLabelSelectorFromMatchers(matchers []string) (string, error) {
	parsed, err := parseMatchers(matchers)
	if err != nil {
		return "", fmt.Errorf("parsing label selector: %w", err)
	}
	return matchersToLabelSelector(parsed), nil
}

func buildLabelSelectorAndProfileType(labelSelector, profileTypeID string) (string, error) {
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

func findReport(r querybackendv1.ReportType, reports []*querybackendv1.Report) *querybackendv1.Report {
	for _, x := range reports {
		if x.ReportType == r {
			return x
		}
	}
	return nil
}
