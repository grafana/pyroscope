package query_backend

import (
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_SERIES_LABELS,
		queryv1.ReportType_REPORT_SERIES_LABELS,
		querySeriesLabels,
		newSeriesLabelsAggregator,
		[]block.Section{block.SectionTSDB}...,
	)
}

func querySeriesLabels(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	m, err := getSeriesLabels(q.ds.Index(), q.req.matchers)
	if err != nil {
		return nil, err
	}
	series := make([]*typesv1.Labels, len(m))
	var i int
	for _, s := range m {
		series[i] = &typesv1.Labels{Labels: s.labels}
		i++
	}
	resp := &queryv1.Report{
		SeriesLabels: &queryv1.SeriesLabelsReport{
			Query:        query.SeriesLabels.CloneVT(),
			SeriesLabels: series,
		},
	}
	return resp, nil
}

type seriesLabelsAggregator struct {
	init   sync.Once
	query  *queryv1.SeriesLabelsQuery
	series *phlaremodel.LabelMerger
}

func newSeriesLabelsAggregator(*queryv1.InvokeRequest) aggregator {
	return new(seriesLabelsAggregator)
}

func (a *seriesLabelsAggregator) aggregate(report *queryv1.Report) error {
	r := report.SeriesLabels
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
		a.series = phlaremodel.NewLabelMerger()
	})
	a.series.MergeSeries(r.SeriesLabels)
	return nil
}

func (a *seriesLabelsAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		SeriesLabels: &queryv1.SeriesLabelsReport{
			Query:        a.query,
			SeriesLabels: a.series.Labels(),
		},
	}
}
