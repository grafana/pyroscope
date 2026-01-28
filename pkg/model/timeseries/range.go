package timeseries

import (
	"sort"

	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// RangeSeries aggregates profiles into series.
// Series contains points spaced by step from start to end.
// Profiles from the same step are aggregated into one point.
func RangeSeries(it iter.Iterator[Value], start, end, step int64, aggregation *typesv1.TimeSeriesAggregationType) []*typesv1.Series {
	return rangeSeriesWithLimit(it, start, end, step, aggregation, DefaultMaxExemplarsPerPoint)
}

// rangeSeriesWithLimit is an internal function that allows specifying maxExemplarsPerPoint.
func rangeSeriesWithLimit(it iter.Iterator[Value], start, end, step int64, aggregation *typesv1.TimeSeriesAggregationType, maxExemplarsPerPoint int) []*typesv1.Series {
	defer it.Close()
	seriesMap := make(map[uint64]*typesv1.Series)
	aggregators := make(map[uint64]Aggregator)

	if !it.Next() {
		return nil
	}

	// advance from the start to the end, adding each step results to the map.
Outer:
	for currentStep := start; currentStep <= end; currentStep += step {
		for {
			point := it.At()
			aggregator, ok := aggregators[point.LabelsHash]
			if !ok {
				aggregator = NewAggregatorWithLimit(aggregation, maxExemplarsPerPoint)
				aggregators[point.LabelsHash] = aggregator
			}
			if point.Ts > currentStep {
				if !aggregator.IsEmpty() {
					series := seriesMap[point.LabelsHash]
					series.Points = append(series.Points, aggregator.GetAndReset())
				}
				break // no more profiles for the currentStep
			}
			// find or create series
			series, ok := seriesMap[point.LabelsHash]
			if !ok {
				seriesMap[point.LabelsHash] = &typesv1.Series{
					Labels: point.Lbs,
					Points: []*typesv1.Point{},
				}
				aggregator.Add(currentStep, &point)
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Aggregate point if it is in the current step.
			if aggregator.GetTimestamp() == currentStep {
				aggregator.Add(currentStep, &point)
				if !it.Next() {
					break Outer
				}
				continue
			}
			// Next step is missing
			if !aggregator.IsEmpty() {
				series.Points = append(series.Points, aggregator.GetAndReset())
			}
			aggregator.Add(currentStep, &point)
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
		return phlaremodel.CompareLabelPairs(series[i].Labels, series[j].Labels) < 0
	})
	return series
}

// selectTopNExemplarsProto selects the top-N exemplars by value.
func selectTopNExemplarsProto(exemplars []*typesv1.Exemplar, maxExemplars int) []*typesv1.Exemplar {
	if len(exemplars) <= maxExemplars {
		return exemplars
	}

	sort.Slice(exemplars, func(i, j int) bool {
		return exemplars[i].Value > exemplars[j].Value
	})
	return exemplars[:maxExemplars]
}
