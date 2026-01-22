package heatmap

import (
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

// HeatmapValue represents a single heatmap point value
type HeatmapValue struct {
	Timestamp     int64
	Value         int64
	ProfileId     int64
	SpanId        uint64
	AttributeRefs []int64
}

// HeatmapSeriesIterator iterates over points in a heatmap series
type HeatmapSeriesIterator struct {
	points []*queryv1.HeatmapPoint
	curr   HeatmapValue
}

// NewHeatmapSeriesIterator creates a new iterator for a heatmap series
func NewHeatmapSeriesIterator(points []*queryv1.HeatmapPoint) *HeatmapSeriesIterator {
	return &HeatmapSeriesIterator{
		points: points,
	}
}

// Next advances the iterator to the next point
func (it *HeatmapSeriesIterator) Next() bool {
	if len(it.points) == 0 {
		return false
	}
	p := it.points[0]
	it.points = it.points[1:]
	it.curr.Timestamp = p.Timestamp
	it.curr.Value = p.Value
	it.curr.ProfileId = p.ProfileId
	it.curr.SpanId = p.SpanId
	it.curr.AttributeRefs = p.AttributeRefs
	return true
}

// At returns the current heatmap value
func (it *HeatmapSeriesIterator) At() HeatmapValue {
	return it.curr
}

// Err returns any error encountered during iteration
func (it *HeatmapSeriesIterator) Err() error {
	return nil
}

// Close closes the iterator
func (it *HeatmapSeriesIterator) Close() error {
	return nil
}

// NewHeatmapReportIterator creates an iterator from multiple heatmap reports
func NewHeatmapReportIterator(reports []*queryv1.HeatmapReport) iter.Iterator[HeatmapValue] {
	if len(reports) == 0 {
		return iter.NewEmptyIterator[HeatmapValue]()
	}

	var iters []iter.Iterator[HeatmapValue]
	for _, report := range reports {
		if report == nil {
			continue
		}
		for _, series := range report.HeatmapSeries {
			if series != nil && len(series.Points) > 0 {
				iters = append(iters, NewHeatmapSeriesIterator(series.Points))
			}
		}
	}

	if len(iters) == 0 {
		return iter.NewEmptyIterator[HeatmapValue]()
	}

	if len(iters) == 1 {
		return iters[0]
	}

	// Create a merge iterator that sorts by timestamp, then spanID, then profileID
	return newHeatmapMergeIterator(iters...)
}

func newHeatmapMergeIterator(iters ...iter.Iterator[HeatmapValue]) iter.Iterator[HeatmapValue] {
	// For now, return a simple iterator if we only have one
	// A full merge iterator implementation would be more complex
	if len(iters) == 1 {
		return iters[0]
	}

	// TODO: Implement proper k-way merge using loser tree
	// For now, concatenate iterators (not ideal for sorted merging)
	return &concatenatingIterator{iters: iters}
}

// concatenatingIterator concatenates multiple iterators
type concatenatingIterator struct {
	iters   []iter.Iterator[HeatmapValue]
	current int
}

func (it *concatenatingIterator) Next() bool {
	for it.current < len(it.iters) {
		if it.iters[it.current].Next() {
			return true
		}
		it.current++
	}
	return false
}

func (it *concatenatingIterator) At() HeatmapValue {
	if it.current < len(it.iters) {
		return it.iters[it.current].At()
	}
	return HeatmapValue{}
}

func (it *concatenatingIterator) Err() error {
	if it.current < len(it.iters) {
		return it.iters[it.current].Err()
	}
	return nil
}

func (it *concatenatingIterator) Close() error {
	var lastErr error
	for _, iter := range it.iters {
		if err := iter.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
