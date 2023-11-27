package model

import (
	"sort"
	"sync"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// SumSeries combines (merges) multiple series into one. Samples with matching timestamps will be summed up.
func SumSeries(series ...[]*typesv1.Series) []*typesv1.Series {
	m := NewSumSeriesMerger()
	for _, s := range series {
		m.MergeSeries(s)
	}
	return m.Series()
}

// MergeSeries combines (merges) multiple series into one.
// Depending on the aggregation type, it will either sum up, average or discard samples with matching timestamps.
func MergeSeries(aggregation *typesv1.TimeSeriesAggregationType, series ...[]*typesv1.Series) []*typesv1.Series {
	m := newSeriesMerger(aggregation)
	for _, s := range series {
		m.MergeSeries(s)
	}
	return m.Series()
}

type SeriesMerger struct {
	mu         sync.Mutex
	series     map[uint64]*typesv1.Series
	aggregator TimeSeriesAggregator
}

// NewFirstValueSeriesMerger creates a merger that discards samples with matching timestamps
func NewFirstValueSeriesMerger() *SeriesMerger {
	aggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_FIRST_VALUE
	return newSeriesMerger(&aggregation)
}

// NewSumSeriesMerger creates a merger that sums up samples with matching timestamps
func NewSumSeriesMerger() *SeriesMerger {
	aggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
	return newSeriesMerger(&aggregation)
}

// NewAvgSeriesMerger creates a merger that averages samples with matching timestamps
func NewAvgSeriesMerger() *SeriesMerger {
	aggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
	return newSeriesMerger(&aggregation)
}

func newSeriesMerger(aggregation *typesv1.TimeSeriesAggregationType) *SeriesMerger {
	return &SeriesMerger{
		series:     make(map[uint64]*typesv1.Series),
		aggregator: NewTimeSeriesAggregator(aggregation),
	}
}

func (m *SeriesMerger) MergeSeries(s []*typesv1.Series) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range s {
		h := Labels(x.Labels).Hash()
		d, ok := m.series[h]
		if !ok {
			m.series[h] = x
			continue
		}
		d.Points = append(d.Points, x.Points...)
	}
}

func (m *SeriesMerger) Series() []*typesv1.Series {
	if len(m.series) == 0 {
		return nil
	}
	r := make([]*typesv1.Series, len(m.series))
	var i int
	for _, s := range m.series {
		s.Points = s.Points[:m.mergePoints(s.Points)]
		r[i] = s
		i++
	}
	sort.Slice(r, func(i, j int) bool {
		return CompareLabelPairs(r[i].Labels, r[j].Labels) < 0
	})
	return r
}

func (m *SeriesMerger) mergePoints(points []*typesv1.Point) int {
	l := len(points)
	if l < 2 {
		return l
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})
	var j int
	for i := 0; i < l; i++ {
		if m.aggregator.IsEmpty() {
			m.aggregator.Add(points[i].Timestamp, points[i].Value)
			continue
		}
		if m.aggregator.GetTimestamp() != points[i].Timestamp {
			points[j] = m.aggregator.GetAndReset()
			j++
			m.aggregator.Add(points[i].Timestamp, points[i].Value)
			continue
		}
		m.aggregator.Add(points[i].Timestamp, points[i].Value)
	}
	if !m.aggregator.IsEmpty() {
		points[j] = m.aggregator.GetAndReset()
	}
	return j + 1
}

type TimeSeriesAggregator interface {
	Add(ts int64, value float64)
	GetAndReset() *typesv1.Point
	IsEmpty() bool
	GetTimestamp() int64
}

func NewTimeSeriesAggregator(aggregation *typesv1.TimeSeriesAggregationType) TimeSeriesAggregator {
	if aggregation == nil || *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM {
		return &sumTimeSeriesAggregator{
			ts: -1,
		}
	}
	if *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE {
		return &avgTimeSeriesAggregator{
			ts: -1,
		}
	} else if *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_FIRST_VALUE {
		return &firstValueTimeSeriesAggregator{
			ts: -1,
		}
	} else {
		return &sumTimeSeriesAggregator{
			ts: -1,
		}
	}
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

func (a *sumTimeSeriesAggregator) IsEmpty() bool {
	return a.ts == -1
}

func (a *sumTimeSeriesAggregator) GetTimestamp() int64 {
	return a.ts
}

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

func (a *avgTimeSeriesAggregator) IsEmpty() bool {
	return a.ts == -1
}

func (a *avgTimeSeriesAggregator) GetTimestamp() int64 {
	return a.ts
}

type firstValueTimeSeriesAggregator struct {
	ts    int64
	value float64
}

func (a *firstValueTimeSeriesAggregator) Add(ts int64, value float64) {
	if a.IsEmpty() {
		a.ts = ts
		a.value = value
	}
}

func (a *firstValueTimeSeriesAggregator) GetAndReset() *typesv1.Point {
	tsCopy := a.ts
	valueCopy := a.value
	a.ts = -1
	a.value = 0
	return &typesv1.Point{
		Timestamp: tsCopy,
		Value:     valueCopy,
	}
}

func (a *firstValueTimeSeriesAggregator) IsEmpty() bool {
	return a.ts == -1
}

func (a *firstValueTimeSeriesAggregator) GetTimestamp() int64 {
	return a.ts
}
