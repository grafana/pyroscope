package querybackend

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/model/heatmap"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_HEATMAP,
		queryv1.ReportType_REPORT_HEATMAP,
		queryHeatmap,
		newHeatmapAggregator,
		true,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

func rowsIndividual(q *queryContext, b *heatmap.Builder) error {
	entries, err := profileEntryIterator(q, withAllLabels(), withFetchPartition(false), withFetchProfileIDs(true))
	if err != nil {
		return err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	totalValue, err := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), strings.Split("TotalValue", "."))
	if err != nil {
		return err
	}

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(
		q.ctx,
		entries,
		q.ds.Profiles().RowGroups(),
		bigBatchSize,
		totalValue.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	for rows.Next() {
		row := rows.At()
		b.Add(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			row.Row.ID,
			0,
			row.Values[0][0].Int64(),
		)
	}
	if err = rows.Err(); err != nil {
		return err
	}

	return nil
}

func rowsSpans(q *queryContext, b *heatmap.Builder) (err error) {
	entries, err := profileEntryIterator(q, withAllLabels(), withFetchPartition(false))
	if err != nil {
		return err
	}

	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")
	var columns schemav1.SampleColumns
	if err := columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		return err
	}

	// no span column
	if !columns.HasSpanID() {
		return nil
	}

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(
		q.ctx,
		entries,
		q.ds.Profiles().RowGroups(),
		bigBatchSize,
		columns.SpanID.ColumnIndex,
		columns.Value.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	for rows.Next() {
		row := rows.At()

		for idx := range row.Values[0] {
			b.Add(
				row.Row.Fingerprint,
				row.Row.Labels,
				int64(row.Row.Timestamp),
				"",
				row.Values[0][idx].Uint64(),
				row.Values[1][idx].Int64(),
			)
		}

	}
	if err = rows.Err(); err != nil {
		return err
	}

	return nil
}

func queryHeatmap(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	// Determine if exemplars should be included based on type
	var includeExemplars bool
	switch query.Heatmap.ExemplarType {
	case typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED,
		typesv1.ExemplarType_EXEMPLAR_TYPE_NONE:
		includeExemplars = false
	case typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL:
		if query.Heatmap.QueryType != querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_INDIVIDUAL {
			return nil, fmt.Errorf("individual exemplars only available for individual query type")
		}
		includeExemplars = true
	case typesv1.ExemplarType_EXEMPLAR_TYPE_SPAN:
		if query.Heatmap.QueryType != querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_SPAN {
			return nil, fmt.Errorf("span exemplars only available for span query type")
		}
		includeExemplars = true
	default:
		return nil, fmt.Errorf("unknown exemplar type: %v", query.Heatmap.ExemplarType)
	}

	otelSpan := trace.SpanFromContext(q.ctx)
	otelSpan.SetAttributes(
		attribute.Bool("exemplars.enabled", includeExemplars),
		attribute.String("exemplars.type", query.Heatmap.ExemplarType.String()),
	)

	b := heatmap.NewBuilder(query.Heatmap.GroupBy)

	// Select the appropriate row iterator based on query type
	switch query.Heatmap.QueryType {
	case querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_INDIVIDUAL:
		if err := rowsIndividual(q, b); err != nil {
			return nil, err
		}
	case querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_SPAN:
		if err := rowsSpans(q, b); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("valid heatmap query type must be specified")
	}

	resp := &queryv1.Report{}
	resp.Heatmap = b.Build(resp.Heatmap)

	return resp, nil
}

type heatmapAggregator struct {
	init      sync.Once
	startTime int64
	endTime   int64
	query     *queryv1.HeatmapQuery
	heatmap   *heatmap.Merger
}

func newHeatmapAggregator(req *queryv1.InvokeRequest) aggregator {
	return &heatmapAggregator{
		startTime: req.StartTime,
		endTime:   req.EndTime,
	}
}

func (a *heatmapAggregator) aggregate(report *queryv1.Report) error {
	r := report.Heatmap
	a.init.Do(func() {
		a.heatmap = heatmap.NewMerger(true)
		a.query = r.Query.CloneVT()
	})
	a.heatmap.MergeHeatmap(r)
	return nil
}

func (a *heatmapAggregator) build() *queryv1.Report {
	// Get the merged heatmap report
	mergedReport := a.heatmap.Build()
	mergedReport.Query = a.query

	return &queryv1.Report{
		Heatmap: mergedReport,
	}
}
