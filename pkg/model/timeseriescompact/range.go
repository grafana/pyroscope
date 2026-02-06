package timeseriescompact

import (
	"sort"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

// RangeSeries aggregates compact points into time steps.
func RangeSeries(it iter.Iterator[CompactValue], start, end, step int64) []*queryv1.Series {
	defer it.Close()

	seriesMap := make(map[string]*queryv1.Series)
	aggregators := make(map[string]*Aggregator)
	var seriesRefs map[string][]int64

	if !it.Next() {
		return nil
	}

	seriesRefs = make(map[string][]int64)

	// Advance from start to end, aggregating each step
Outer:
	for currentStep := start; currentStep <= end; currentStep += step {
		for {
			point := it.At()
			key := point.SeriesKey

			agg, ok := aggregators[key]
			if !ok {
				agg = &Aggregator{}
				aggregators[key] = agg
				seriesRefs[key] = point.SeriesRefs
			}

			if point.Ts > currentStep {
				// Flush aggregators that have data
				for k, a := range aggregators {
					if !a.IsEmpty() {
						s, exists := seriesMap[k]
						if !exists {
							s = &queryv1.Series{AttributeRefs: seriesRefs[k]}
							seriesMap[k] = s
						}
						s.Points = append(s.Points, a.GetAndReset())
					}
				}
				break
			}

			// Find or create series
			if _, ok := seriesMap[key]; !ok {
				seriesMap[key] = &queryv1.Series{
					AttributeRefs: point.SeriesRefs,
					Points:        []*queryv1.Point{},
				}
			}

			// Aggregate point if in current step
			if agg.Timestamp() == currentStep || agg.IsEmpty() {
				agg.Add(currentStep, &point)
				if !it.Next() {
					break Outer
				}
				continue
			}

			// Step changed, flush and start new
			s := seriesMap[key]
			if !agg.IsEmpty() {
				s.Points = append(s.Points, agg.GetAndReset())
			}
			agg.Add(currentStep, &point)
			if !it.Next() {
				break Outer
			}
		}
	}

	// Flush remaining aggregators
	for key, agg := range aggregators {
		if !agg.IsEmpty() {
			s, exists := seriesMap[key]
			if !exists {
				s = &queryv1.Series{AttributeRefs: seriesRefs[key]}
				seriesMap[key] = s
			}
			s.Points = append(s.Points, agg.GetAndReset())
		}
	}

	// Sort series by key for deterministic output
	keys := make([]string, 0, len(seriesMap))
	for k := range seriesMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]*queryv1.Series, 0, len(seriesMap))
	for _, k := range keys {
		if s := seriesMap[k]; len(s.Points) > 0 {
			result = append(result, s)
		}
	}

	return result
}
