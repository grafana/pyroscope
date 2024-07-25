package querybackend

import (
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/querybackend/block"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_TIME_SERIES,
		querybackendv1.ReportType_REPORT_TIME_SERIES,
		queryTimeSeries,
		newTimeSeriesAggregator,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

func queryTimeSeries(q *queryContext, query *querybackendv1.Query) (r *querybackendv1.Report, err error) {
	entries, err := profileEntryIterator(q, query.TimeSeries.GroupBy...)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	column, err := schemav1.ResolveColumnByPath(q.svc.Profiles().Schema(), strings.Split("TotalValue", "."))
	if err != nil {
		return nil, err
	}

	rows := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.svc.Profiles().RowGroups(), column.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	builder := phlaremodel.NewTimeSeriesBuilder(query.TimeSeries.GroupBy...)
	for rows.Next() {
		row := rows.At()
		builder.Add(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			float64(row.Values[0][0].Int64()),
		)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	resp := &querybackendv1.Report{
		TimeSeries: &querybackendv1.TimeSeriesReport{
			Query:      query.TimeSeries.CloneVT(),
			TimeSeries: builder.Build(),
		},
	}

	return resp, nil
}

type timeSeriesAggregator struct {
	init      sync.Once
	startTime int64
	endTime   int64
	query     *querybackendv1.TimeSeriesQuery
	series    *phlaremodel.TimeSeriesMerger
}

func newTimeSeriesAggregator(req *querybackendv1.InvokeRequest) aggregator {
	return &timeSeriesAggregator{
		startTime: req.StartTime,
		endTime:   req.EndTime,
	}
}

func (a *timeSeriesAggregator) aggregate(report *querybackendv1.Report) error {
	r := report.TimeSeries
	a.init.Do(func() {
		a.series = phlaremodel.NewTimeSeriesMerger(true)
		a.query = r.Query.CloneVT()
	})
	a.series.MergeTimeSeries(r.TimeSeries)
	return nil
}

func (a *timeSeriesAggregator) build() *querybackendv1.Report {
	// TODO(kolesnikovae): Average aggregation should be implemented in
	//  the way that it can be distributed (count + sum), and should be done
	//  at "aggregate" call.
	sum := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
	return &querybackendv1.Report{
		TimeSeries: &querybackendv1.TimeSeriesReport{
			Query: a.query,
			TimeSeries: phlaremodel.RangeSeries(phlaremodel.NewTimeSeriesMergeIterator(a.series.TimeSeries()),
				a.startTime,
				a.endTime,
				int64(a.query.GetStep()),
				&sum),
		},
	}
}
