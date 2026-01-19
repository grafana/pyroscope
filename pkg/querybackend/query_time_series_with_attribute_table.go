package querybackend

import (
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/runutil"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_TIME_SERIES_WITH_ATTRIBUTE_TABLE,
		queryv1.ReportType_REPORT_TIME_SERIES_WITH_ATTRIBUTE_TABLE,
		queryTimeSeriesWithAttributeTable,
		newTimeSeriesWithAttributeTableAggregator,
		true,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

func queryTimeSeriesWithAttributeTable(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	includeExemplars, err := validateExemplarType(query.TimeSeriesWithAttributeTable.ExemplarType)
	if err != nil {
		return nil, err
	}

	span := opentracing.SpanFromContext(q.ctx)
	span.SetTag("exemplars.enabled", includeExemplars)
	span.SetTag("exemplars.type", query.TimeSeriesWithAttributeTable.ExemplarType.String())

	opts := []profileIteratorOption{
		withFetchPartition(false),
	}

	if includeExemplars {
		opts = append(opts,
			withAllLabels(),
			withFetchProfileIDs(true),
		)
	} else {
		opts = append(opts,
			withGroupByLabels(query.TimeSeriesWithAttributeTable.GroupBy...),
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

	builder := phlaremodel.NewTimeSeriesBuilder(query.TimeSeriesWithAttributeTable.GroupBy...)
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
			row.Row.ID,
		)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	var timeSeries []*queryv1.Series
	var attributeTable *queryv1.AttributeTable
	if includeExemplars {
		timeSeries = builder.BuildWithAttributeTable()
		attributeTable = builder.AttributeTable().Build(nil)
		span.SetTag("exemplars.raw_count", builder.ExemplarCount())
	} else {
		typesV1Series := builder.Build()
		timeSeries = make([]*queryv1.Series, len(typesV1Series))
		for i, s := range typesV1Series {
			points := make([]*queryv1.Point, len(s.Points))
			for j, p := range s.Points {
				points[j] = &queryv1.Point{
					Value:       p.Value,
					Timestamp:   p.Timestamp,
					Annotations: p.Annotations,
				}
			}
			timeSeries[i] = &queryv1.Series{
				Labels: s.Labels,
				Points: points,
			}
		}
	}

	resp := &queryv1.Report{
		TimeSeriesWithAttributeTable: &queryv1.TimeSeriesWithAttributeTableReport{
			Query:          query.TimeSeriesWithAttributeTable.CloneVT(),
			TimeSeries:     timeSeries,
			AttributeTable: attributeTable,
		},
	}

	return resp, nil
}

type timeSeriesWithAttributeTableAggregator struct {
	init      sync.Once
	startTime int64
	endTime   int64
	query     *queryv1.TimeSeriesQuery
	series    *attributetable.SeriesMerger
}

func newTimeSeriesWithAttributeTableAggregator(req *queryv1.InvokeRequest) aggregator {
	return &timeSeriesWithAttributeTableAggregator{
		startTime: req.StartTime,
		endTime:   req.EndTime,
	}
}

func (a *timeSeriesWithAttributeTableAggregator) aggregate(report *queryv1.Report) error {
	r := report.TimeSeriesWithAttributeTable
	a.init.Do(func() {
		a.series = attributetable.NewSeriesMerger(true)
		a.query = r.Query.CloneVT()
	})

	a.series.MergeWithAttributeTable(r.TimeSeries, r.AttributeTable)
	return nil
}

func (a *timeSeriesWithAttributeTableAggregator) build() *queryv1.Report {
	sum := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
	stepMilli := time.Duration(a.query.GetStep() * float64(time.Second)).Milliseconds()

	labelBasedSeries := a.series.ExpandToFullLabels()

	seriesIterator := phlaremodel.NewTimeSeriesMergeIterator(labelBasedSeries)
	rangedSeries := phlaremodel.RangeSeries(
		seriesIterator,
		a.startTime,
		a.endTime,
		stepMilli,
		&sum,
	)

	// Convert back to query.v1.Series with attribute_refs
	attrTable := attributetable.NewTable()
	timeSeries := make([]*queryv1.Series, len(rangedSeries))
	for i, s := range rangedSeries {
		points := make([]*queryv1.Point, len(s.Points))
		for j, p := range s.Points {
			if len(p.Exemplars) == 0 {
				points[j] = &queryv1.Point{
					Value:       p.Value,
					Timestamp:   p.Timestamp,
					Annotations: p.Annotations,
				}
				continue
			}

			// Convert labels back to attribute_refs
			exemplars := make([]*queryv1.Exemplar, len(p.Exemplars))
			for k, ex := range p.Exemplars {
				refs := attrTable.Refs(ex.Labels, nil)
				exemplars[k] = &queryv1.Exemplar{
					Timestamp:     ex.Timestamp,
					ProfileId:     ex.ProfileId,
					SpanId:        ex.SpanId,
					Value:         ex.Value,
					AttributeRefs: refs,
				}
			}
			points[j] = &queryv1.Point{
				Value:       p.Value,
				Timestamp:   p.Timestamp,
				Annotations: p.Annotations,
				Exemplars:   exemplars,
			}
		}
		timeSeries[i] = &queryv1.Series{
			Labels: s.Labels,
			Points: points,
		}
	}

	return &queryv1.Report{
		TimeSeriesWithAttributeTable: &queryv1.TimeSeriesWithAttributeTableReport{
			Query:          a.query,
			TimeSeries:     timeSeries,
			AttributeTable: attrTable.Build(nil),
		},
	}
}
