package querybackend

import (
	"sync"
	"time"

	"github.com/grafana/dskit/runutil"
	"github.com/parquet-go/parquet-go"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_TIME_SERIES,
		queryv1.ReportType_REPORT_TIME_SERIES,
		queryTimeSeries,
		newTimeSeriesAggregator,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

func queryTimeSeries(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	// TODO(XXXXX: remove this override
	v := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
	query.TimeSeries.Aggregation = &v

	entries, err := profileEntryIterator(q, query.TimeSeries.GroupBy...)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	totalValueColumn, err := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), []string{schemav1.TotalValueColumnName})
	if err != nil {
		return nil, err
	}

	idColumn, err := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), []string{schemav1.IDColumnName})
	if err != nil {
		return nil, err
	}

	// these columns might not be present
	annotationKeysColumn, _ := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), schemav1.AnnotationKeyColumnPath)
	annotationValuesColumn, _ := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), schemav1.AnnotationValueColumnPath)

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(
		q.ctx,
		entries,
		q.ds.Profiles().RowGroups(),
		bigBatchSize,
		totalValueColumn.ColumnIndex,
		annotationKeysColumn.ColumnIndex,
		annotationValuesColumn.ColumnIndex,
		idColumn.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	builder := phlaremodel.NewTimeSeriesBuilder(query.TimeSeries.GroupBy...)
	for rows.Next() {
		row := rows.At()
		annotations := schemav1.Annotations{
			Keys:   make([]string, 0),
			Values: make([]string, 0),
		}

		var id []byte
		for _, e := range row.Values {
			if e[0].Column() == annotationKeysColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Keys = append(annotations.Keys, e[0].String())
			}
			if e[0].Column() == annotationValuesColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Values = append(annotations.Values, e[0].String())
			}
			if e[0].Column() == idColumn.ColumnIndex && e[0].Kind() == parquet.FixedLenByteArray {
				id = e[0].ByteArray()
			}
		}
		builder.AddWithProfileID(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			float64(row.Values[0][0].Int64()),
			id,
			annotations,
		)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	resp := &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
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
	query     *queryv1.TimeSeriesQuery
	series    *phlaremodel.TimeSeriesMerger
}

func newTimeSeriesAggregator(req *queryv1.InvokeRequest) aggregator {
	return &timeSeriesAggregator{
		startTime: req.StartTime,
		endTime:   req.EndTime,
	}
}

func (a *timeSeriesAggregator) aggregate(report *queryv1.Report) error {
	r := report.TimeSeries
	isSum :=
		r.Query.Aggregation.Type == nil || *r.Query.Aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM

	a.init.Do(func() {
		a.series = phlaremodel.NewTimeSeriesMerger(isSum)
		a.query = r.Query.CloneVT()
	})
	a.series.MergeTimeSeries(r.TimeSeries)
	return nil
}

func (a *timeSeriesAggregator) build() *queryv1.Report {
	var aggregationType = a.query.Aggregation
	if aggregationType == nil {
		// TODO(XXXXX: remove this override
		v := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
		aggregationType = &v
	}

	stepMilli := time.Duration(a.query.GetStep() * float64(time.Second)).Milliseconds()
	seriesIterator := phlaremodel.NewTimeSeriesMergeIterator(a.series.TimeSeries())
	return &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
			Query: a.query,
			TimeSeries: phlaremodel.RangeSeries(
				seriesIterator,
				a.startTime,
				a.endTime,
				stepMilli,
				aggregationType,
			),
		},
	}
}
