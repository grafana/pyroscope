package querybackend

import (
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/runutil"
	"github.com/parquet-go/parquet-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	includeExemplars, err := validateExemplarType(query.TimeSeries.ExemplarType)
	if err != nil {
		return nil, err
	}

	opts := []profileIteratorOption{
		withFetchPartition(false), // Partition data not needed, as we don't access stacktraces at all
	}

	if includeExemplars {
		opts = append(opts,
			withAllLabels(),
			withFetchProfileIDs(true),
		)
	} else {
		opts = append(opts,
			withGroupByLabels(query.TimeSeries.GroupBy...),
		)
	}

	entries, err := profileEntryIterator(q, opts...)
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

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(
		q.ctx,
		entries,
		q.ds.Profiles().RowGroups(),
		bigBatchSize,
		column.ColumnIndex,
		annotationKeysColumn.ColumnIndex,
		annotationValuesColumn.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	builder := phlaremodel.NewTimeSeriesBuilder(query.TimeSeries.GroupBy...)
	for rows.Next() {
		row := rows.At()
		annotations := schemav1.Annotations{
			Keys:   make([]string, 0),
			Values: make([]string, 0),
		}
		for _, e := range row.Values {
			if e[0].Column() == annotationKeysColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Keys = append(annotations.Keys, e[0].String())
			}
			if e[0].Column() == annotationValuesColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Values = append(annotations.Values, e[0].String())
			}
		}

		builder.Add(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			float64(row.Values[0][0].Int64()),
			annotations,
			row.Row.ID, // Profile ID comes from ProfileEntry now
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

	return &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
			Query:      a.query,
			TimeSeries: series,
		},
	}
}

// validateExemplarType validates the exemplar type and returns whether to include exemplars.
func validateExemplarType(exemplarType typesv1.ExemplarType) (bool, error) {
	switch exemplarType {
	case typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED,
		typesv1.ExemplarType_EXEMPLAR_TYPE_NONE:
		return false, nil
	case typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL:
		return true, nil
	case typesv1.ExemplarType_EXEMPLAR_TYPE_SPAN:
		return false, status.Error(codes.Unimplemented, "exemplar type span is not implemented")
	default:
		return false, status.Errorf(codes.InvalidArgument, "unknown exemplar type: %v", exemplarType)
	}
}
