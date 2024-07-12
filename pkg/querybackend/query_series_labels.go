package querybackend

import (
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_SERIES_LABELS,
		querybackendv1.ReportType_REPORT_SERIES_LABELS,
		func(q *queryContext) queryHandler { return q.querySeriesLabels },
		func() reportMerger { return new(seriesLabelsMerger) },
	)
}

func (q *queryContext) querySeriesLabels(query *querybackendv1.Query) (*querybackendv1.Report, error) {
	// TODO: implement
	resp := &querybackendv1.Report{
		SeriesLabels: &querybackendv1.SeriesLabelsReport{
			Query: query.SeriesLabels.CloneVT(),
			SeriesLabels: []*typesv1.Labels{{Labels: []*typesv1.LabelPair{
				{Name: "service_name", Value: "service_name"},
				{Name: "__profile_type__", Value: "__profile_type__"},
				{Name: "__type__", Value: "__type__"},
				{Name: "__name__", Value: "__name__"},
			}}},
		},
	}
	return resp, nil
}

type seriesLabelsMerger struct {
	init   sync.Once
	query  *querybackendv1.SeriesLabelsQuery
	series *model.LabelMerger
}

func (m *seriesLabelsMerger) merge(report *querybackendv1.Report) error {
	r := report.SeriesLabels
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.series = model.NewLabelMerger()
	})
	m.series.MergeSeries(r.SeriesLabels)
	return nil
}

func (m *seriesLabelsMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.series == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: querybackendv1.ReportType_REPORT_SERIES_LABELS,
		SeriesLabels: &querybackendv1.SeriesLabelsReport{
			Query:        m.query,
			SeriesLabels: m.series.SeriesLabels(),
		},
	})
}
