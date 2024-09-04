package query_frontend

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// TODO(kolesnikovae): Extend the metastore API to query arbitrary dataset labels.

var profileTypeLabels2 = []string{
	"__profile_type__",
	"service_name",
}

var profileTypeLabels5 = []string{
	"__name__",
	"__profile_type__",
	"__type__",
	"pyroscope_app",
	"service_name",
}

func isProfileTypeQuery(labels, matchers []string) bool {
	if len(matchers) > 0 {
		return false
	}
	var s []string
	switch len(labels) {
	case 2:
		s = profileTypeLabels2
	case 5:
		s = profileTypeLabels5
	default:
		return false
	}
	sort.Strings(labels)
	return slices.Compare(s, labels) == 0
}

func listProfileTypesFromMetadataAsSeriesLabels(
	ctx context.Context,
	client *metastoreclient.Client,
	tenants []string,
	startTime int64,
	endTime int64,
	labels []string,
) (*connect.Response[querierv1.SeriesResponse], error) {
	resp, err := listProfileTypesFromMetadata(ctx, client, tenants, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&querierv1.SeriesResponse{
		LabelsSet: resp.buildSeriesLabels(labels),
	}), nil
}

func listProfileTypesFromMetadata(
	ctx context.Context,
	client *metastoreclient.Client,
	tenants []string,
	startTime int64,
	endTime int64,
) (*ptypes, error) {
	md, err := client.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: startTime,
		EndTime:   endTime,
		Query:     "{}",
	})
	if err != nil {
		return nil, err
	}
	p := newProfileTypesResponseBuilder(len(md.Blocks) * 8)
	for _, m := range md.Blocks {
		for _, s := range m.Datasets {
			p.addServiceProfileTypes(s.Name, s.ProfileTypes...)
		}
	}
	return p, nil
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

type ptypes struct {
	services map[string]map[string]struct{}
}

func newProfileTypesResponseBuilder(size int) *ptypes {
	return &ptypes{services: make(map[string]map[string]struct{}, size)}
}

func (p *ptypes) addServiceProfileTypes(s string, types ...string) {
	sp, ok := p.services[s]
	if !ok {
		sp = make(map[string]struct{}, len(types))
		p.services[s] = sp
	}
	for _, t := range types {
		sp[t] = struct{}{}
	}
}

func (p *ptypes) buildSeriesLabels(names []string) (labels []*typesv1.Labels) {
	switch len(names) {
	case 2:
		labels = p.buildSeriesLabels2()
	case 5:
		labels = p.buildSeriesLabels5()
	default:
		panic("bug: invalid request: expected 2 or 5 label names")
	}
	slices.SortFunc(labels, func(a, b *typesv1.Labels) int {
		return phlaremodel.CompareLabelPairs(a.Labels, b.Labels)
	})
	return labels
}

func (p *ptypes) buildSeriesLabels2() []*typesv1.Labels {
	labels := make([]*typesv1.Labels, 0, len(p.services)*4)
	for n, types := range p.services {
		for t := range types {
			labels = append(labels, &typesv1.Labels{
				Labels: []*typesv1.LabelPair{
					{Name: "__profile_type__", Value: t},
					{Name: "service_name", Value: n},
				},
			})
		}
	}
	return labels
}

func (p *ptypes) buildSeriesLabels5() []*typesv1.Labels {
	labels := make([]*typesv1.Labels, 0, len(p.services)*4)
	for n, types := range p.services {
		for t := range types {
			pt, err := phlaremodel.ParseProfileTypeSelector(t)
			if err != nil {
				panic("bug: invalid profile type: " + err.Error())
			}
			labels = append(labels, &typesv1.Labels{
				Labels: []*typesv1.LabelPair{
					{Name: "__profile_type__", Value: t},
					{Name: "service_name", Value: n},
					{Name: "__name__", Value: pt.Name},
					{Name: "__type__", Value: pt.SampleType},
				},
			})
		}
	}
	return labels
}

//nolint:unused
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
		for _, x := range b.Datasets {
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
