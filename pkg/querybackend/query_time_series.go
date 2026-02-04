package querybackend

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/runutil"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
	"github.com/grafana/pyroscope/pkg/model/timeseries"
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
	registerQueryType(
		queryv1.QueryType_QUERY_TIME_SERIES_COMPACT,
		queryv1.ReportType_REPORT_TIME_SERIES_COMPACT,
		queryTimeSeriesCompact,
		newTimeSeriesCompactAggregator,
		true,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

type timeSeriesQueryResult struct {
	series        []*typesv1.Series
	exemplarCount int
}

// executeTimeSeriesQuery is shared by both query types to avoid duplication.
func executeTimeSeriesQuery(q *queryContext, groupBy []string, exemplarType typesv1.ExemplarType) (*timeSeriesQueryResult, error) {
	includeExemplars, err := validateExemplarType(exemplarType)
	if err != nil {
		return nil, err
	}

	span := opentracing.SpanFromContext(q.ctx)
	span.SetTag("exemplars.enabled", includeExemplars)
	span.SetTag("exemplars.type", exemplarType.String())

	opts := []profileIteratorOption{withFetchPartition(false)}
	if includeExemplars {
		opts = append(opts, withAllLabels(), withFetchProfileIDs(true))
	} else {
		opts = append(opts, withGroupByLabels(groupBy...))
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

	rows := parquetquery.NewRepeatedRowIteratorBatchSize(q.ctx, entries, q.ds.Profiles().RowGroups(), bigBatchSize, column.ColumnIndex, annotationKeysColumn.ColumnIndex, annotationValuesColumn.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	builder := timeseries.NewBuilder(groupBy...)
	for rows.Next() {
		row := rows.At()
		annotations := schemav1.Annotations{Keys: make([]string, 0), Values: make([]string, 0)}
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

	var series []*typesv1.Series
	if includeExemplars {
		series = builder.BuildWithExemplars()
	} else {
		series = builder.Build()
	}

	return &timeSeriesQueryResult{series: series, exemplarCount: builder.ExemplarCount()}, nil
}

func queryTimeSeries(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	result, err := executeTimeSeriesQuery(q, query.TimeSeries.GroupBy, query.TimeSeries.ExemplarType)
	if err != nil {
		return nil, err
	}

	if result.exemplarCount > 0 {
		span := opentracing.SpanFromContext(q.ctx)
		span.SetTag("exemplars.raw_count", result.exemplarCount)
	}

	return &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
			Query:      query.TimeSeries.CloneVT(),
			TimeSeries: result.series,
		},
	}, nil
}

func queryTimeSeriesCompact(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	result, err := executeTimeSeriesQuery(q, query.TimeSeriesCompact.GroupBy, query.TimeSeriesCompact.ExemplarType)
	if err != nil {
		return nil, err
	}

	if result.exemplarCount > 0 {
		span := opentracing.SpanFromContext(q.ctx)
		span.SetTag("exemplars.raw_count", result.exemplarCount)
	}

	at := attributetable.New()
	series := make([]*queryv1.Series, len(result.series))
	for i, s := range result.series {
		series[i] = &queryv1.Series{
			AttributeRefs: at.Refs(phlaremodel.Labels(s.Labels), nil),
			Points:        convertPoints(s.Points, at),
		}
	}

	return &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query:          query.TimeSeriesCompact.CloneVT(),
			TimeSeries:     series,
			AttributeTable: at.Build(nil),
		},
	}, nil
}

func convertPoints(points []*typesv1.Point, at *attributetable.Table) []*queryv1.Point {
	result := make([]*queryv1.Point, len(points))
	for i, p := range points {
		result[i] = &queryv1.Point{Value: p.Value, Timestamp: p.Timestamp}
		if len(p.Annotations) > 0 {
			keys := make([]string, len(p.Annotations))
			values := make([]string, len(p.Annotations))
			for j, a := range p.Annotations {
				keys[j] = a.Key
				values[j] = a.Value
			}
			result[i].AnnotationRefs = at.AnnotationRefs(keys, values, nil)
		}
		if len(p.Exemplars) > 0 {
			result[i].Exemplars = make([]*queryv1.Exemplar, len(p.Exemplars))
			for j, ex := range p.Exemplars {
				result[i].Exemplars[j] = &queryv1.Exemplar{
					Timestamp:     ex.Timestamp,
					ProfileId:     ex.ProfileId,
					SpanId:        ex.SpanId,
					Value:         ex.Value,
					AttributeRefs: at.Refs(phlaremodel.Labels(ex.Labels), nil),
				}
			}
		}
	}
	return result
}

type timeSeriesAggregator struct {
	init      sync.Once
	startTime int64
	endTime   int64
	query     *queryv1.TimeSeriesQuery
	series    *timeseries.Merger
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
		a.series = timeseries.NewMerger(true)
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
	seriesIterator := timeseries.NewTimeSeriesMergeIterator(a.series.TimeSeries())
	series := timeseries.RangeSeries(seriesIterator, a.startTime, a.endTime, stepMilli, &sum)
	return &queryv1.Report{
		TimeSeries: &queryv1.TimeSeriesReport{
			Query:      a.query,
			TimeSeries: series,
		},
	}
}

type timeSeriesCompactAggregator struct {
	init      sync.Once
	startTime int64
	endTime   int64
	query     *queryv1.TimeSeriesQuery
	merger    *timeSeriesCompactMerger
}

func newTimeSeriesCompactAggregator(req *queryv1.InvokeRequest) aggregator {
	return &timeSeriesCompactAggregator{
		startTime: req.StartTime,
		endTime:   req.EndTime,
	}
}

func (a *timeSeriesCompactAggregator) aggregate(report *queryv1.Report) error {
	r := report.TimeSeriesCompact
	a.init.Do(func() {
		a.merger = newTimeSeriesCompactMerger(true)
		a.query = r.Query.CloneVT()
	})
	a.merger.Merge(r)
	return nil
}

func (a *timeSeriesCompactAggregator) build() *queryv1.Report {
	stepMilli := time.Duration(a.query.GetStep() * float64(time.Second)).Milliseconds()
	report := a.merger.Build(a.startTime, a.endTime, stepMilli)
	report.Query = a.query
	return &queryv1.Report{TimeSeriesCompact: report}
}

// timeSeriesCompactMerger merges time series reports in compact format.
type timeSeriesCompactMerger struct {
	mu       sync.Mutex
	atMerger *attributetable.Merger
	sum      bool
	series   map[string]*compactSeries
}

type compactSeries struct {
	refs   []int64
	points []*queryv1.Point
}

func newTimeSeriesCompactMerger(sum bool) *timeSeriesCompactMerger {
	return &timeSeriesCompactMerger{
		atMerger: attributetable.NewMerger(),
		sum:      sum,
		series:   make(map[string]*compactSeries),
	}
}

// Merge adds a report to the merger, remapping attribute refs to the merged table.
func (m *timeSeriesCompactMerger) Merge(r *queryv1.TimeSeriesCompactReport) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r == nil || len(r.TimeSeries) == 0 {
		return
	}

	m.atMerger.Merge(r.AttributeTable, func(remap *attributetable.Remapper) {
		for _, s := range r.TimeSeries {
			refs := remap.Refs(s.AttributeRefs)
			key := seriesKey(refs)

			existing, ok := m.series[key]
			if !ok {
				existing = &compactSeries{refs: refs}
				m.series[key] = existing
			}

			existing.points = slices.Grow(existing.points, len(s.Points))
			for _, p := range s.Points {
				pt := &queryv1.Point{Timestamp: p.Timestamp, Value: p.Value}
				if len(p.AnnotationRefs) > 0 {
					pt.AnnotationRefs = remap.Refs(p.AnnotationRefs)
				}
				if len(p.Exemplars) > 0 {
					pt.Exemplars = make([]*queryv1.Exemplar, len(p.Exemplars))
					for i, ex := range p.Exemplars {
						pt.Exemplars[i] = &queryv1.Exemplar{
							Timestamp:     ex.Timestamp,
							ProfileId:     ex.ProfileId,
							SpanId:        ex.SpanId,
							Value:         ex.Value,
							AttributeRefs: remap.Refs(ex.AttributeRefs),
						}
					}
				}
				existing.points = append(existing.points, pt)
			}
		}
	})
}

// Build returns the merged report with step-based aggregation applied.
func (m *timeSeriesCompactMerger) Build(start, end, step int64) *queryv1.TimeSeriesCompactReport {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.series) == 0 {
		return &queryv1.TimeSeriesCompactReport{}
	}

	keys := make([]string, 0, len(m.series))
	for k := range m.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]*queryv1.Series, 0, len(m.series))
	for _, k := range keys {
		s := m.series[k]
		sort.Slice(s.points, func(i, j int) bool {
			return s.points[i].Timestamp < s.points[j].Timestamp
		})

		// Apply step-based aggregation
		rangedPoints := m.rangePoints(s.points, start, end, step)
		if len(rangedPoints) == 0 {
			continue
		}

		result = append(result, &queryv1.Series{
			AttributeRefs: s.refs,
			Points:        rangedPoints,
		})
	}

	return &queryv1.TimeSeriesCompactReport{
		TimeSeries:     result,
		AttributeTable: m.atMerger.BuildAttributeTable(nil),
	}
}

// rangePoints aggregates points into time steps, similar to timeseries.RangeSeries but operating on compact format.
func (m *timeSeriesCompactMerger) rangePoints(points []*queryv1.Point, start, end, step int64) []*queryv1.Point {
	if len(points) == 0 || step <= 0 {
		return nil
	}

	var result []*queryv1.Point
	var agg *compactPointAggregator
	pointIdx := 0

	for currentStep := start; currentStep <= end; currentStep += step {
		// Aggregate all points that fall into this step
		for pointIdx < len(points) && points[pointIdx].Timestamp <= currentStep {
			if agg == nil {
				agg = &compactPointAggregator{sum: m.sum}
			}
			agg.Add(currentStep, points[pointIdx])
			pointIdx++
		}

		if agg != nil && !agg.IsEmpty() {
			result = append(result, agg.GetAndReset())
		}
	}

	return result
}

type compactPointAggregator struct {
	sum            bool
	ts             int64
	value          float64
	annotationRefs []int64
	exemplars      []*queryv1.Exemplar
	hasData        bool
}

func (a *compactPointAggregator) Add(ts int64, p *queryv1.Point) {
	a.ts = ts
	if a.sum {
		a.value += p.Value
	} else {
		a.value = p.Value
	}
	a.annotationRefs = append(a.annotationRefs, p.AnnotationRefs...)
	if len(p.Exemplars) > 0 {
		a.exemplars = mergeExemplars(a.exemplars, p.Exemplars)
	}
	a.hasData = true
}

func (a *compactPointAggregator) GetAndReset() *queryv1.Point {
	pt := &queryv1.Point{
		Timestamp: a.ts,
		Value:     a.value,
	}
	if len(a.annotationRefs) > 0 {
		pt.AnnotationRefs = dedupeAnnotationRefs(a.annotationRefs)
	}
	if len(a.exemplars) > 0 {
		pt.Exemplars = selectTopNExemplars(a.exemplars, timeseries.DefaultMaxExemplarsPerPoint)
	}

	a.ts = 0
	a.value = 0
	a.annotationRefs = nil
	a.exemplars = nil
	a.hasData = false

	return pt
}

func (a *compactPointAggregator) IsEmpty() bool {
	return !a.hasData
}

func dedupeAnnotationRefs(refs []int64) []int64 {
	if len(refs) <= 1 {
		return refs
	}
	slices.Sort(refs)
	j := 0
	for i := 1; i < len(refs); i++ {
		if refs[j] != refs[i] {
			j++
			refs[j] = refs[i]
		}
	}
	return refs[:j+1]
}

func selectTopNExemplars(exemplars []*queryv1.Exemplar, n int) []*queryv1.Exemplar {
	if len(exemplars) <= n {
		return exemplars
	}
	sort.Slice(exemplars, func(i, j int) bool {
		return exemplars[i].Value > exemplars[j].Value
	})
	return exemplars[:n]
}

// mergeExemplars combines two exemplar lists.
// For exemplars with the same profileID, it keeps the highest value and intersects attribute refs.
func mergeExemplars(a, b []*queryv1.Exemplar) []*queryv1.Exemplar {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	type exemplarGroup struct {
		exemplar *queryv1.Exemplar
		refSets  [][]int64
	}
	byProfileID := make(map[string]*exemplarGroup, len(a)+len(b))

	for _, ex := range a {
		byProfileID[ex.ProfileId] = &exemplarGroup{
			exemplar: ex,
			refSets:  [][]int64{ex.AttributeRefs},
		}
	}

	for _, ex := range b {
		existing, found := byProfileID[ex.ProfileId]
		if !found {
			byProfileID[ex.ProfileId] = &exemplarGroup{
				exemplar: ex,
				refSets:  [][]int64{ex.AttributeRefs},
			}
		} else {
			if ex.Value > existing.exemplar.Value {
				existing.exemplar = ex
			}
			existing.refSets = append(existing.refSets, ex.AttributeRefs)
		}
	}

	result := make([]*queryv1.Exemplar, 0, len(byProfileID))
	for _, group := range byProfileID {
		ex := group.exemplar
		if len(group.refSets) > 1 {
			intersected := intersectRefs(group.refSets)
			if intersected == nil && ex.AttributeRefs != nil {
				ex.AttributeRefs = []int64{}
			} else {
				ex.AttributeRefs = intersected
			}
		}
		result = append(result, ex)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ProfileId < result[j].ProfileId })
	return result
}

// intersectRefs returns the intersection of multiple ref slices.
func intersectRefs(refSets [][]int64) []int64 {
	if len(refSets) == 0 {
		return nil
	}
	if len(refSets) == 1 {
		return refSets[0]
	}

	// Build set from first
	set := make(map[int64]struct{}, len(refSets[0]))
	for _, ref := range refSets[0] {
		set[ref] = struct{}{}
	}

	// Intersect with remaining
	for i := 1; i < len(refSets); i++ {
		newSet := make(map[int64]struct{})
		for _, ref := range refSets[i] {
			if _, ok := set[ref]; ok {
				newSet[ref] = struct{}{}
			}
		}
		set = newSet
	}

	if len(set) == 0 {
		return nil
	}

	// Convert back to sorted slice
	result := make([]int64, 0, len(set))
	for ref := range set {
		result = append(result, ref)
	}
	slices.Sort(result)
	return result
}

func seriesKey(refs []int64) string {
	var sb strings.Builder
	for i, ref := range refs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatInt(ref, 16))
	}
	return sb.String()
}

func validateExemplarType(exemplarType typesv1.ExemplarType) (bool, error) {
	switch exemplarType {
	case typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE:
		return false, nil
	case typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL:
		return true, nil
	case typesv1.ExemplarType_EXEMPLAR_TYPE_SPAN:
		return false, status.Error(codes.Unimplemented, "exemplar type span is not implemented")
	default:
		return false, status.Errorf(codes.InvalidArgument, "unknown exemplar type: %v", exemplarType)
	}
}
