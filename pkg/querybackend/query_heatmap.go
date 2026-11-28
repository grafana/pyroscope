package querybackend

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"
	"github.com/parquet-go/parquet-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/iter"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/heatmap"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
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

func rowsIndividual(q *queryContext, b *heatmap.Builder) (err error) {
	totalValue, err := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), strings.Split("TotalValue", "."))
	if err != nil {
		return err
	}

	entries, err := profileEntryIterator(
		q,
		withAllLabels(),
		withFetchPartition(false),
		withFetchProfileIDs(true),
		withExcludeSampled(),
		withPrefetchColumns(totalValue.ColumnIndex),
	)
	if err != nil {
		return err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

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
			model.TraceID{},
			row.Values[0][0].Int64(),
		)
	}
	if err = rows.Err(); err != nil {
		return err
	}

	return nil
}

func rowsSpans(q *queryContext, b *heatmap.Builder) (err error) {
	var columns schemav1.SampleColumns
	if err := columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		return err
	}

	// no span column
	if !columns.HasSpanID() {
		return nil
	}

	ranges := spanRowRanges(q.ds.Profiles().RowGroups(), columns.SpanID.ColumnIndex)
	if len(ranges) == 0 {
		return nil
	}
	indices := []int{
		columns.SpanID.ColumnIndex,
		columns.Value.ColumnIndex,
	}
	if columns.HasTraceID() {
		indices = append(indices, columns.TraceID.ColumnIndex)
	}

	entries, err := profileEntryIterator(q, withAllLabels(), withFetchPartition(false), withExcludeSampled(), withPrefetchColumns(indices...))
	if err != nil {
		return err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")
	filtered := &spanProfileIterator{Iterator: entries, ranges: ranges}

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(
		q.ctx,
		filtered,
		q.ds.Profiles().RowGroups(),
		bigBatchSize,
		indices...,
	)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	for rows.Next() {
		row := rows.At()

		for idx := range row.Values[0] {
			var traceID model.TraceID
			if columns.HasTraceID() {
				traceIDBytes := row.Values[2][idx].Bytes()
				if len(traceIDBytes) == len(traceID) {
					copy(traceID[:], traceIDBytes)
				}
			}
			b.Add(
				row.Row.Fingerprint,
				row.Row.Labels,
				int64(row.Row.Timestamp),
				"",
				row.Values[0][idx].Uint64(),
				traceID,
				row.Values[1][idx].Int64(),
			)
		}

	}
	if err = rows.Err(); err != nil {
		return err
	}

	return nil
}

// spanRowRanges preserves global row numbers: removing row groups themselves
// would shift the offsets used by the repeated-row reader.
func spanRowRanges(groups []parquet.RowGroup, column int) [][2]int64 {
	var ranges [][2]int64
	var start int64
	for _, rg := range groups {
		end := start + rg.NumRows()
		ci, err := rg.ColumnChunks()[column].ColumnIndex()
		if err != nil || columnMayHaveSpans(ci) {
			ranges = append(ranges, [2]int64{start, end})
		}
		start = end
	}
	return ranges
}

func columnMayHaveSpans(ci parquet.ColumnIndex) bool {
	if ci == nil || ci.NumPages() == 0 {
		return true // No index is not evidence of absent spans.
	}
	for p := 0; p < ci.NumPages(); p++ {
		if ci.NullPage(p) {
			continue
		}
		min, max := ci.MinValue(p), ci.MaxValue(p)
		if min.IsNull() || max.IsNull() || min.Uint64() != 0 || max.Uint64() != 0 {
			return true
		}
	}
	return false
}

type spanProfileIterator struct {
	iter.Iterator[ProfileEntry]
	ranges [][2]int64
}

func (s *spanProfileIterator) Next() bool {
	for s.Iterator.Next() {
		rn := s.At().RowNum
		for len(s.ranges) > 0 && rn >= s.ranges[0][1] {
			s.ranges = s.ranges[1:]
		}
		if len(s.ranges) == 0 {
			return false
		}
		if rn >= s.ranges[0][0] {
			return true
		}
	}
	return false
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
