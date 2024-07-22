package frontend

import (
	"context"
	"slices"
	"sort"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("matchers", c.Msg.Matchers).
		SetTag("label_names", c.Msg.LabelNames)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSeriesProcedure)
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	interval, ok := phlaremodel.GetTimeRange(c.Msg)
	if ok {
		validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, interval, model.Now())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if validated.IsEmpty {
			return connect.NewResponse(&querierv1.SeriesResponse{}), nil
		}
		c.Msg.Start = int64(validated.Start)
		c.Msg.End = int64(validated.End)
	}

	if isProfileTypeQuery(c.Msg.LabelNames, c.Msg.Matchers) {
		_ = level.Info(f.log).Log("msg", "listing profile types from metadata as series labels")
		return f.listProfileTypesFromMetadataAsSeriesLabels(ctx, tenantIDs, c.Msg.Start, c.Msg.End, c.Msg.LabelNames)
	}

	labelSelector, err := buildLabelSelectorFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := f.invoke(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType: querybackendv1.QueryType_QUERY_SERIES_LABELS,
		SeriesLabels: &querybackendv1.SeriesLabelsQuery{
			LabelNames: c.Msg.LabelNames,
		},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}
	return connect.NewResponse(&querierv1.SeriesResponse{
		LabelsSet: report.SeriesLabels.SeriesLabels,
	}), nil
}

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

func (f *Frontend) listProfileTypesFromMetadataAsSeriesLabels(
	ctx context.Context, tenants []string, startTime, endTime int64, labels []string,
) (*connect.Response[querierv1.SeriesResponse], error) {
	resp, err := f.listProfileTypesFromMetadata(ctx, tenants, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&querierv1.SeriesResponse{
		LabelsSet: resp.buildSeriesLabels(labels),
	}), nil
}

func (f *Frontend) listProfileTypesFromMetadata(
	ctx context.Context, tenants []string, startTime, endTime int64,
) (*ptypes, error) {
	metas, err := f.listMetadata(ctx, tenants, startTime, endTime, "{}")
	if err != nil {
		return nil, err
	}
	p := newProfileTypesResponseBuilder(len(metas) * 8)
	for _, m := range metas {
		for _, s := range m.TenantServices {
			p.addServiceProfileTypes(s.Name, s.ProfileTypes...)
		}
	}
	return p, nil
}

type ptypes struct {
	services map[string]map[string]struct{}
}

func newProfileTypesResponseBuilder(size int) *ptypes {
	return &ptypes{
		services: make(map[string]map[string]struct{}, size),
	}
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
		for t, _ := range types {
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
		for t, _ := range types {
			pt, err := phlaremodel.ParseProfileTypeSelector(t)
			if err != nil {
				panic(err)
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
