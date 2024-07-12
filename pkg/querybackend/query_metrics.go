package querybackend

import (
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_METRICS,
		querybackendv1.ReportType_REPORT_METRICS,
		func(q *queryContext) queryHandler { return q.queryMetrics },
		func() reportMerger { return new(metricsMerger) },
	)
}

func (q *queryContext) queryMetrics(query *querybackendv1.Query) (*querybackendv1.Report, error) {
	// TODO: implement
	resp := &querybackendv1.Report{
		Metrics: &querybackendv1.MetricsReport{
			Query:   query.Metrics.CloneVT(),
			Metrics: []*typesv1.Series{},
		},
	}
	return resp, nil
}

type metricsMerger struct {
	init    sync.Once
	query   *querybackendv1.MetricsQuery
	metrics *model.MetricsMerger
}

func (m *metricsMerger) merge(report *querybackendv1.Report) error {
	r := report.Metrics
	m.init.Do(func() {
		sum := r.Query.GetAggregation() == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
		m.metrics = model.NewMetricsMerger(sum)
		m.query = r.Query.CloneVT()
	})
	m.metrics.MergeMetrics(r.Metrics)
	return nil
}

func (m *metricsMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.metrics == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: querybackendv1.ReportType_REPORT_METRICS,
		Metrics: &querybackendv1.MetricsReport{
			Query:   m.query,
			Metrics: m.metrics.Metrics(),
		},
	})
}
