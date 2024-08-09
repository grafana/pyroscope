package model

import (
	"math"
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

type TimeSeriesValue struct {
	Ts         int64
	Lbs        []*typesv1.LabelPair
	LabelsHash uint64
	Value      float64
}

func (p TimeSeriesValue) Labels() Labels        { return p.Lbs }
func (p TimeSeriesValue) Timestamp() model.Time { return model.Time(p.Ts) }

type TimeSeriesIterator struct {
	point []*typesv1.Point
	curr  TimeSeriesValue
}

func NewSeriesIterator(lbs []*typesv1.LabelPair, points []*typesv1.Point) *TimeSeriesIterator {
	return &TimeSeriesIterator{
		point: points,

		curr: TimeSeriesValue{
			Lbs:        lbs,
			LabelsHash: Labels(lbs).Hash(),
		},
	}
}

func (s *TimeSeriesIterator) Next() bool {
	if len(s.point) == 0 {
		return false
	}
	p := s.point[0]
	s.point = s.point[1:]
	s.curr.Ts = p.Timestamp
	s.curr.Value = p.Value
	return true
}

func (s *TimeSeriesIterator) At() TimeSeriesValue { return s.curr }
func (s *TimeSeriesIterator) Err() error          { return nil }
func (s *TimeSeriesIterator) Close() error        { return nil }

func NewTimeSeriesMergeIterator(series []*typesv1.Series) iter.Iterator[TimeSeriesValue] {
	iters := make([]iter.Iterator[TimeSeriesValue], 0, len(series))
	for _, s := range series {
		iters = append(iters, NewSeriesIterator(s.Labels, s.Points))
	}
	return NewMergeIterator(TimeSeriesValue{Ts: math.MaxInt64}, false, iters...)
}

type TimeSeriesAggregator interface {
	Add(ts int64, value float64)
	GetAndReset() *typesv1.Point
	IsEmpty() bool
	GetTimestamp() int64
}

func NewTimeSeriesAggregator(aggregation *typesv1.TimeSeriesAggregationType) TimeSeriesAggregator {
	if aggregation == nil {
		return &sumTimeSeriesAggregator{ts: -1}
	}
	if *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE {
		return &avgTimeSeriesAggregator{ts: -1}
	}
	return &sumTimeSeriesAggregator{ts: -1}
}

type sumTimeSeriesAggregator struct {
	ts  int64
	sum float64
}

func (a *sumTimeSeriesAggregator) Add(ts int64, value float64) {
	a.ts = ts
	a.sum += value
}

func (a *sumTimeSeriesAggregator) GetAndReset() *typesv1.Point {
	tsCopy := a.ts
	sumCopy := a.sum
	a.ts = -1
	a.sum = 0
	return &typesv1.Point{
		Timestamp: tsCopy,
		Value:     sumCopy,
	}
}

func (a *sumTimeSeriesAggregator) IsEmpty() bool       { return a.ts == -1 }
func (a *sumTimeSeriesAggregator) GetTimestamp() int64 { return a.ts }

type avgTimeSeriesAggregator struct {
	ts    int64
	sum   float64
	count int64
}

func (a *avgTimeSeriesAggregator) Add(ts int64, value float64) {
	a.ts = ts
	a.sum += value
	a.count++
}

func (a *avgTimeSeriesAggregator) GetAndReset() *typesv1.Point {
	avg := a.sum / float64(a.count)
	tsCopy := a.ts
	a.ts = -1
	a.sum = 0
	a.count = 0
	return &typesv1.Point{
		Timestamp: tsCopy,
		Value:     avg,
	}
}

func (a *avgTimeSeriesAggregator) IsEmpty() bool       { return a.ts == -1 }
func (a *avgTimeSeriesAggregator) GetTimestamp() int64 { return a.ts }

// RangeSeries aggregates profiles into series.
// Series contains points spaced by step from start to end.
// Profiles from the same step are aggregated into one point.
func RangeSeries(it iter.Iterator[TimeSeriesValue], start, end, step int64, aggregation *typesv1.TimeSeriesAggregationType) []*typesv1.Series {
	defer it.Close()
	seriesMap := make(map[uint64]*typesv1.Series)
	aggregators := make(map[uint64]TimeSeriesAggregator)

	if !it.Next() {
		return nil
	}

	// advance from the start to the end, adding each step results to the map.
Outer:
	for currentStep := start; currentStep <= end; currentStep += step {
		for {
			aggregator, ok := aggregators[it.At().LabelsHash]
			if !ok {
				aggregator = NewTimeSeriesAggregator(aggregation)
				aggregators[it.At().LabelsHash] = aggregator
			}
			if it.At().Ts > currentStep {
				if !aggregator.IsEmpty() {
					series := seriesMap[it.At().LabelsHash]
					series.Points = append(series.Points, aggregator.GetAndReset())
				}
				break // no more profiles for the currentStep
			}
			// find or create series
			series, ok := seriesMap[it.At().LabelsHash]
			if !ok {
				seriesMap[it.At().LabelsHash] = &typesv1.Series{
					Labels: it.At().Lbs,
					Points: []*typesv1.Point{},
				}
				aggregator.Add(currentStep, it.At().Value)
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Aggregate point if it is in the current step.
			if aggregator.GetTimestamp() == currentStep {
				aggregator.Add(currentStep, it.At().Value)
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Next step is missing
			if !aggregator.IsEmpty() {
				series.Points = append(series.Points, aggregator.GetAndReset())
			}
			aggregator.Add(currentStep, it.At().Value)
			if !it.Next() {
				break Outer
			}
		}
	}
	for lblHash, aggregator := range aggregators {
		if !aggregator.IsEmpty() {
			seriesMap[lblHash].Points = append(seriesMap[lblHash].Points, aggregator.GetAndReset())
		}
	}
	series := lo.Values(seriesMap)
	sort.Slice(series, func(i, j int) bool {
		return CompareLabelPairs(series[i].Labels, series[j].Labels) < 0
	})
	return series
}
