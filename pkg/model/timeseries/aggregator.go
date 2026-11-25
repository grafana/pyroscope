package timeseries

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

// DefaultMaxExemplarsPerPoint is the default maximum number of exemplars tracked per point.
// TODO: make it configurable via tenant limits.
const DefaultMaxExemplarsPerPoint = 1

type Aggregator interface {
	Add(ts int64, point *Value)
	GetAndReset() *typesv1.Point
	IsEmpty() bool
	GetTimestamp() int64
}

func NewAggregator(aggregation *typesv1.TimeSeriesAggregationType) Aggregator {
	return NewAggregatorWithLimit(aggregation, DefaultMaxExemplarsPerPoint)
}

func NewAggregatorWithLimit(aggregation *typesv1.TimeSeriesAggregationType, maxExemplarsPerPoint int) Aggregator {
	if aggregation == nil {
		return &sumAggregator{ts: -1, maxExemplarsPerPoint: maxExemplarsPerPoint}
	}
	if *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE {
		return &avgAggregator{ts: -1, maxExemplarsPerPoint: maxExemplarsPerPoint}
	}
	return &sumAggregator{ts: -1, maxExemplarsPerPoint: maxExemplarsPerPoint}
}

// annotationAccumulator deduplicates key/value pairs on insertion, bounding
// memory for repeated annotations while preserving first-seen order.
type annotationAccumulator struct {
	points []*typesv1.ProfileAnnotation
	seen   map[annotationKey]struct{}
}

type annotationKey struct{ key, value string }

func (a *annotationAccumulator) add(annotations []*typesv1.ProfileAnnotation) {
	for _, annotation := range annotations {
		key := annotationKey{annotation.Key, annotation.Value}
		if _, exists := a.seen[key]; exists {
			continue
		}
		if a.seen == nil {
			a.seen = make(map[annotationKey]struct{})
		}
		a.seen[key] = struct{}{}
		a.points = append(a.points, annotation)
	}
}

func (a *annotationAccumulator) getAndReset() []*typesv1.ProfileAnnotation {
	points := make([]*typesv1.ProfileAnnotation, len(a.points))
	copy(points, a.points)
	clear(a.points)
	a.points = a.points[:0]
	clear(a.seen)
	return points
}

type sumAggregator struct {
	ts                   int64
	sum                  float64
	annotations          annotationAccumulator
	exemplars            []*typesv1.Exemplar
	exemplarByID         map[string]exemplarEntry
	maxExemplarsPerPoint int
}

type exemplarEntry struct {
	idx       int
	labelSets []phlaremodel.Labels
}

func addExemplarsProto(dst *[]*typesv1.Exemplar, byID *map[string]exemplarEntry, exemplars []*typesv1.Exemplar) {
	if *byID == nil {
		*byID = make(map[string]exemplarEntry, len(exemplars))
	}
	m := *byID
	for _, ex := range exemplars {
		entry, found := m[ex.ProfileId]
		if !found {
			m[ex.ProfileId] = exemplarEntry{
				idx: len(*dst),
			}
			*dst = append(*dst, ex)
			continue
		}
		existing := (*dst)[entry.idx]
		if len(entry.labelSets) == 0 {
			entry.labelSets = []phlaremodel.Labels{existing.Labels, ex.Labels}
		} else {
			entry.labelSets = append(entry.labelSets, ex.Labels)
		}
		if ex.Value > existing.Value {
			(*dst)[entry.idx] = ex
		}
		m[ex.ProfileId] = entry
	}
}

func finalizeExemplarsProto(exemplars []*typesv1.Exemplar, byID map[string]exemplarEntry, maxExemplars int) []*typesv1.Exemplar {
	top := selectTopNExemplarsProto(exemplars, maxExemplars)
	for i, ex := range top {
		if entry := byID[ex.ProfileId]; len(entry.labelSets) > 1 {
			top[i] = &typesv1.Exemplar{
				Timestamp: ex.Timestamp,
				ProfileId: ex.ProfileId,
				SpanId:    ex.SpanId,
				TraceId:   ex.TraceId,
				Value:     ex.Value,
				Labels:    phlaremodel.IntersectAll(entry.labelSets),
			}
		}
	}
	return top
}

func (a *sumAggregator) Add(ts int64, point *Value) {
	a.ts = ts
	a.sum += point.Value
	a.annotations.add(point.Annotations)

	if len(point.Exemplars) > 0 {
		addExemplarsProto(&a.exemplars, &a.exemplarByID, point.Exemplars)
	}
}

func (a *sumAggregator) GetAndReset() *typesv1.Point {
	tsCopy := a.ts
	sumCopy := a.sum
	annotationsCopy := a.annotations.getAndReset()

	var exemplars []*typesv1.Exemplar
	if len(a.exemplars) > 0 {
		exemplars = finalizeExemplarsProto(a.exemplars, a.exemplarByID, a.maxExemplarsPerPoint)
	}

	a.ts = -1
	a.sum = 0
	clear(a.exemplars)
	a.exemplars = a.exemplars[:0]
	clear(a.exemplarByID)

	return &typesv1.Point{
		Timestamp:   tsCopy,
		Value:       sumCopy,
		Annotations: annotationsCopy,
		Exemplars:   exemplars,
	}
}

func (a *sumAggregator) IsEmpty() bool       { return a.ts == -1 }
func (a *sumAggregator) GetTimestamp() int64 { return a.ts }

type avgAggregator struct {
	ts                   int64
	sum                  float64
	count                int64
	annotations          annotationAccumulator
	exemplars            []*typesv1.Exemplar
	exemplarByID         map[string]exemplarEntry
	maxExemplarsPerPoint int
}

func (a *avgAggregator) Add(ts int64, point *Value) {
	a.ts = ts
	a.sum += point.Value
	a.count++
	a.annotations.add(point.Annotations)

	if len(point.Exemplars) > 0 {
		addExemplarsProto(&a.exemplars, &a.exemplarByID, point.Exemplars)
	}
}

func (a *avgAggregator) GetAndReset() *typesv1.Point {
	avg := a.sum / float64(a.count)
	tsCopy := a.ts
	annotationsCopy := a.annotations.getAndReset()

	var exemplars []*typesv1.Exemplar
	if len(a.exemplars) > 0 {
		exemplars = finalizeExemplarsProto(a.exemplars, a.exemplarByID, a.maxExemplarsPerPoint)
	}

	a.ts = -1
	a.sum = 0
	a.count = 0
	clear(a.exemplars)
	a.exemplars = a.exemplars[:0]
	clear(a.exemplarByID)

	return &typesv1.Point{
		Timestamp:   tsCopy,
		Value:       avg,
		Annotations: annotationsCopy,
		Exemplars:   exemplars,
	}
}

func (a *avgAggregator) IsEmpty() bool       { return a.ts == -1 }
func (a *avgAggregator) GetTimestamp() int64 { return a.ts }
