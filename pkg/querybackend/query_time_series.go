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
	merger    *timeSeriesMerger
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
		a.merger = &timeSeriesMerger{
			atMerger: attributetable.NewMerger(),
			sum:      true,
			series:   make(map[string]*atSeries),
		}
		a.query = r.Query.CloneVT()
	})
	a.merger.merge(r)
	return nil
}

func (a *timeSeriesCompactAggregator) build() *queryv1.Report {
	sum := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
	stepMilli := time.Duration(a.query.GetStep() * float64(time.Second)).Milliseconds()

	typesV1 := a.merger.expand()
	iter := timeseries.NewTimeSeriesMergeIterator(typesV1)
	ranged := timeseries.RangeSeries(iter, a.startTime, a.endTime, stepMilli, &sum)

	at := attributetable.New()
	series := make([]*queryv1.Series, len(ranged))
	for i, s := range ranged {
		series[i] = &queryv1.Series{
			AttributeRefs: at.Refs(phlaremodel.Labels(s.Labels), nil),
			Points:        convertPoints(s.Points, at),
		}
	}

	return &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query:          a.query,
			TimeSeries:     series,
			AttributeTable: at.Build(nil),
		},
	}
}

type timeSeriesMerger struct {
	mu       sync.Mutex
	atMerger *attributetable.Merger
	sum      bool
	series   map[string]*atSeries
}

type atSeries struct {
	refs   []int64
	points []*queryv1.Point
}

func (m *timeSeriesMerger) merge(r *queryv1.TimeSeriesCompactReport) {
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
				existing = &atSeries{refs: refs}
				m.series[key] = existing
			}

			existing.points = slices.Grow(existing.points, len(s.Points))
			for _, p := range s.Points {
				pt := &queryv1.Point{Timestamp: p.Timestamp, Value: p.Value}
				if len(p.AnnotationRefs) > 0 {
					pt.AnnotationRefs = remap.Refs(slices.Clone(p.AnnotationRefs))
				}
				if len(p.Exemplars) > 0 {
					pt.Exemplars = make([]*queryv1.Exemplar, len(p.Exemplars))
					for i, ex := range p.Exemplars {
						pt.Exemplars[i] = &queryv1.Exemplar{
							Timestamp:     ex.Timestamp,
							ProfileId:     ex.ProfileId,
							SpanId:        ex.SpanId,
							Value:         ex.Value,
							AttributeRefs: remap.Refs(slices.Clone(ex.AttributeRefs)),
						}
					}
				}
				existing.points = append(existing.points, pt)
			}
		}
	})
}

func (m *timeSeriesMerger) expand() []*typesv1.Series {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.series) == 0 {
		return nil
	}

	table := m.atMerger.BuildAttributeTable(nil)
	labelMap := make(map[int64]*typesv1.LabelPair, len(table.Keys))
	for i := range table.Keys {
		labelMap[int64(i)] = &typesv1.LabelPair{Name: table.Keys[i], Value: table.Values[i]}
	}

	keys := make([]string, 0, len(m.series))
	for k := range m.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]*typesv1.Series, len(m.series))
	for i, k := range keys {
		s := m.series[k]
		if m.sum {
			s.points = s.points[:m.mergePoints(s.points)]
		} else {
			sort.Slice(s.points, func(i, j int) bool { return s.points[i].Timestamp < s.points[j].Timestamp })
		}

		labels := make([]*typesv1.LabelPair, len(s.refs))
		for j, ref := range s.refs {
			labels[j] = labelMap[ref]
		}

		points := make([]*typesv1.Point, len(s.points))
		for j, p := range s.points {
			points[j] = &typesv1.Point{Value: p.Value, Timestamp: p.Timestamp}
			if len(p.AnnotationRefs) > 0 {
				points[j].Annotations = make([]*typesv1.ProfileAnnotation, len(p.AnnotationRefs))
				for k, ref := range p.AnnotationRefs {
					kv := labelMap[ref]
					points[j].Annotations[k] = &typesv1.ProfileAnnotation{Key: kv.Name, Value: kv.Value}
				}
			}
			if len(p.Exemplars) > 0 {
				points[j].Exemplars = make([]*typesv1.Exemplar, len(p.Exemplars))
				for k, ex := range p.Exemplars {
					exLabels := make([]*typesv1.LabelPair, len(ex.AttributeRefs))
					for l, ref := range ex.AttributeRefs {
						exLabels[l] = labelMap[ref]
					}
					points[j].Exemplars[k] = &typesv1.Exemplar{
						Timestamp: ex.Timestamp,
						ProfileId: ex.ProfileId,
						SpanId:    ex.SpanId,
						Value:     ex.Value,
						Labels:    exLabels,
					}
				}
			}
		}

		result[i] = &typesv1.Series{Labels: labels, Points: points}
	}

	return result
}

func (m *timeSeriesMerger) mergePoints(points []*queryv1.Point) int {
	if len(points) < 2 {
		return len(points)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp < points[j].Timestamp })

	j := 0
	for i := 1; i < len(points); i++ {
		if points[j].Timestamp != points[i].Timestamp {
			j++
			points[j] = points[i]
			continue
		}
		points[j].Value += points[i].Value
		points[j].AnnotationRefs = append(points[j].AnnotationRefs, points[i].AnnotationRefs...)
		points[j].Exemplars = mergeExemplars(points[j].Exemplars, points[i].Exemplars)
	}
	return j + 1
}

func mergeExemplars(a, b []*queryv1.Exemplar) []*queryv1.Exemplar {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	m := make(map[string]*queryv1.Exemplar, len(a)+len(b))
	for _, ex := range a {
		m[ex.ProfileId] = ex
	}
	for _, ex := range b {
		if existing := m[ex.ProfileId]; existing == nil || ex.Value > existing.Value {
			m[ex.ProfileId] = ex
		}
	}
	result := make([]*queryv1.Exemplar, 0, len(m))
	for _, ex := range m {
		result = append(result, ex)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProfileId < result[j].ProfileId })
	return result
}

func seriesKey(refs []int64) string {
	var sb strings.Builder
	for i, ref := range refs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatInt(ref, 10))
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
