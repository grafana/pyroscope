package timeseries

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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

type sumAggregator struct {
	ts                   int64
	sum                  float64
	annotations          []*typesv1.ProfileAnnotation
	exemplars            []*typesv1.Exemplar
	maxExemplarsPerPoint int
}

func (a *sumAggregator) Add(ts int64, point *Value) {
	a.ts = ts
	a.sum += point.Value
	a.annotations = append(a.annotations, point.Annotations...)

	if len(point.Exemplars) > 0 {
		a.exemplars = mergeExemplars(a.exemplars, point.Exemplars)
	}
}

func (a *sumAggregator) GetAndReset() *typesv1.Point {
	tsCopy := a.ts
	sumCopy := a.sum
	annotationsCopy := make([]*typesv1.ProfileAnnotation, len(a.annotations))
	copy(annotationsCopy, a.annotations)

	var exemplars []*typesv1.Exemplar
	if len(a.exemplars) > 0 {
		exemplars = selectTopNExemplarsProto(a.exemplars, a.maxExemplarsPerPoint)
	}

	a.ts = -1
	a.sum = 0
	a.annotations = a.annotations[:0]
	a.exemplars = nil

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
	annotations          []*typesv1.ProfileAnnotation
	exemplars            []*typesv1.Exemplar
	maxExemplarsPerPoint int
}

func (a *avgAggregator) Add(ts int64, point *Value) {
	a.ts = ts
	a.sum += point.Value
	a.count++
	a.annotations = append(a.annotations, point.Annotations...)

	if len(point.Exemplars) > 0 {
		a.exemplars = mergeExemplars(a.exemplars, point.Exemplars)
	}
}

func (a *avgAggregator) GetAndReset() *typesv1.Point {
	avg := a.sum / float64(a.count)
	tsCopy := a.ts
	annotationsCopy := make([]*typesv1.ProfileAnnotation, len(a.annotations))
	copy(annotationsCopy, a.annotations)

	var exemplars []*typesv1.Exemplar
	if len(a.exemplars) > 0 {
		exemplars = selectTopNExemplarsProto(a.exemplars, a.maxExemplarsPerPoint)
	}

	a.ts = -1
	a.sum = 0
	a.count = 0
	a.annotations = a.annotations[:0]
	a.exemplars = nil

	return &typesv1.Point{
		Timestamp:   tsCopy,
		Value:       avg,
		Annotations: annotationsCopy,
		Exemplars:   exemplars,
	}
}

func (a *avgAggregator) IsEmpty() bool       { return a.ts == -1 }
func (a *avgAggregator) GetTimestamp() int64 { return a.ts }
