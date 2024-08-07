package queryfrontend

import (
	"context"
	"slices"
	"sort"

	"connectrpc.com/connect"
	"github.com/go-kit/log"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

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

func IsProfileTypeQuery(labels, matchers []string) bool {
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

func ListProfileTypesFromMetadataAsSeriesLabels(
	ctx context.Context,
	client *metastoreclient.Client,
	logger log.Logger,
	tenants []string,
	startTime, endTime int64,
	labels []string,

) (*connect.Response[querierv1.SeriesResponse], error) {
	resp, err := listProfileTypesFromMetadata(ctx, client, logger, tenants, startTime, endTime)
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
	logger log.Logger,
	tenants []string,
	startTime, endTime int64,
) (*ptypes, error) {
	metas, err := ListMetadata(ctx, client, logger, tenants, startTime, endTime, "{}")
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
