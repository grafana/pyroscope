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
		queryMetrics,
		newMetricsMerger,
	)
}

func queryMetrics(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
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

func newMetricsMerger() reportMerger { return new(metricsMerger) }

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

func (m *metricsMerger) report() *querybackendv1.Report {
	return &querybackendv1.Report{
		Metrics: &querybackendv1.MetricsReport{
			Query:   m.query,
			Metrics: m.metrics.Metrics(),
		},
	}
}
