package querybackend

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
		true,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

func queryTimeSeries(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	entries, err := profileEntryIterator(q)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	column, err := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), strings.Split("TotalValue", "."))
	if err != nil {
		return nil, err
	}

	// these columns might not be present
	annotationKeysColumn, _ := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), schemav1.AnnotationKeyColumnPath)
	annotationValuesColumn, _ := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), schemav1.AnnotationValueColumnPath)

	includeExemplars := query.TimeSeries.ExemplarType == typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL

	columnIndices := []int{
		column.ColumnIndex,
		annotationKeysColumn.ColumnIndex,
		annotationValuesColumn.ColumnIndex,
	}

	var idColumn parquet.LeafColumn
	if includeExemplars {
		idColumn, _ = schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), []string{schemav1.IDColumnName})
		columnIndices = append(columnIndices, idColumn.ColumnIndex)
	}

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(
		q.ctx,
		entries,
		q.ds.Profiles().RowGroups(),
		bigBatchSize,
		columnIndices...,
	)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	builder := phlaremodel.NewTimeSeriesBuilder(query.TimeSeries.GroupBy...)
	for rows.Next() {
		row := rows.At()
		annotations := schemav1.Annotations{
			Keys:   make([]string, 0),
			Values: make([]string, 0),
		}
		var profileID string
		for _, e := range row.Values {
			if e[0].Column() == annotationKeysColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Keys = append(annotations.Keys, e[0].String())
			}
			if e[0].Column() == annotationValuesColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Values = append(annotations.Values, e[0].String())
			}
			if includeExemplars && e[0].Column() == idColumn.ColumnIndex && e[0].Kind() == parquet.UUID().Type().Kind() {
				var u uuid.UUID
				copy(u[:], e[0].ByteArray())
				profileID = u.String()
			}
		}

		builder.Add(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			float64(row.Values[0][0].Int64()),
			annotations,
			profileID,
		)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	var timeSeries []*typesv1.Series
	if includeExemplars {
		timeSeries = builder.BuildWithExemplars()
	} else {
		timeSeries = builder.Build()
	}

	resp := &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
			Query:      query.TimeSeries.CloneVT(),
			TimeSeries: timeSeries,
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
	a.init.Do(func() {
		a.series = phlaremodel.NewTimeSeriesMerger(true)
		a.query = r.Query.CloneVT()
	})
	a.series.MergeTimeSeries(r.TimeSeries)
	return nil
}

func (a *timeSeriesAggregator) build() *queryv1.Report {
	// TODO(kolesnikovae): Average aggregation should be implemented in
	//  the way that it can be distributed (count + sum), and should be done
	//  at "aggregate" call.
	sum := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
	stepMilli := time.Duration(a.query.GetStep() * float64(time.Second)).Milliseconds()
	seriesIterator := phlaremodel.NewTimeSeriesMergeIterator(a.series.TimeSeries())
	series := phlaremodel.RangeSeries(
		seriesIterator,
		a.startTime,
		a.endTime,
		stepMilli,
		&sum,
	)

	if len(a.query.GroupBy) > 0 {
		series = a.filterLabels(series, a.query.GroupBy)
	}

	return &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
			Query:      a.query,
			TimeSeries: series,
		},
	}
}

// filterLabels filters both series labels and exemplar labels based on groupBy.
// Series labels are filtered to only include groupBy labels.
// Exemplar labels are filtered to exclude groupBy labels.
func (a *timeSeriesAggregator) filterLabels(series []*typesv1.Series, groupBy []string) []*typesv1.Series {
	for _, s := range series {
		s.Labels = phlaremodel.Labels(s.Labels).WithLabels(groupBy...)
		for _, point := range s.Points {
			for _, exemplar := range point.Exemplars {
				exemplar.Labels = phlaremodel.FilterNonGroupedLabels(
					exemplar.Labels,
					groupBy,
				)
			}
		}
	}
	return series
}
