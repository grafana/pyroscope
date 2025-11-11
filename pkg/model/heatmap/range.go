package heatmap

import (
	"math"
	"sort"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const (
	// DefaultYAxisBuckets is the default number of Y-axis buckets
	DefaultYAxisBuckets = 20
)

// RangeHeatmap buckets heatmap points into time/value buckets for visualization
func RangeHeatmap(
	reports []*queryv1.HeatmapReport,
	start, end, step int64,
	aggregation *typesv1.TimeSeriesAggregationType,
) []*typesv1.HeatmapSeries {
	if len(reports) == 0 {
		return nil
	}

	// Collect all points and find min/max values
	type pointWithLabels struct {
		timestamp int64
		value     uint64
		labels    []*typesv1.LabelPair
	}
	var allPoints []pointWithLabels
	var minValue, maxValue uint64 = math.MaxUint64, 0

	for _, report := range reports {
		if report == nil {
			continue
		}
		for _, series := range report.HeatmapSeries {
			if series == nil {
				continue
			}
			for _, point := range series.Points {
				if point == nil {
					continue
				}
				allPoints = append(allPoints, pointWithLabels{
					timestamp: point.Timestamp,
					value:     point.Value,
					// TODO: Resolve attribute refs to actual labels
					labels: nil,
				})

				if point.Value < minValue {
					minValue = point.Value
				}
				if point.Value > maxValue {
					maxValue = point.Value
				}
			}
		}
	}

	if len(allPoints) == 0 {
		return nil
	}

	// Handle edge case where all values are the same
	if minValue == maxValue {
		maxValue = minValue + 1
	}

	// Create Y-axis bucket boundaries
	yBuckets := createYAxisBuckets(minValue, maxValue, DefaultYAxisBuckets)

	// Create time buckets
	timeBuckets := createTimeBuckets(start, end, step)

	// Create a map to track counts per (time, y-bucket)
	type cellKey struct {
		timeIdx int
		yIdx    int
	}
	cellCounts := make(map[cellKey]int32)

	// Bucket each point
	for _, point := range allPoints {
		// Find time bucket
		timeIdx := findTimeBucket(point.timestamp, timeBuckets)
		if timeIdx < 0 {
			continue
		}

		// Find value bucket
		yIdx := findValueBucket(point.value, yBuckets)
		if yIdx < 0 {
			continue
		}

		cellCounts[cellKey{timeIdx, yIdx}]++
	}

	// Build slots grouped by timestamp
	slotsMap := make(map[int64]*typesv1.HeatmapSlot)
	for cell, count := range cellCounts {
		timestamp := timeBuckets[cell.timeIdx]
		slot, ok := slotsMap[timestamp]
		if !ok {
			slot = &typesv1.HeatmapSlot{
				Timestamp: timestamp,
				YMin:      make([]float64, len(yBuckets)),
				Counts:    make([]int32, len(yBuckets)),
			}
			// Initialize Y bucket minimums
			for i, bucket := range yBuckets {
				slot.YMin[i] = float64(bucket.min)
			}
			slotsMap[timestamp] = slot
		}
		slot.Counts[cell.yIdx] = count
	}

	// Convert map to sorted slice
	var slots []*typesv1.HeatmapSlot
	for _, slot := range slotsMap {
		slots = append(slots, slot)
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].Timestamp < slots[j].Timestamp
	})

	// For now, return a single series with no labels
	// TODO: Group by series labels if needed
	return []*typesv1.HeatmapSeries{
		{
			Labels: nil,
			Slots:  slots,
		},
	}
}

// yBucket represents a Y-axis bucket
type yBucket struct {
	min uint64
	max uint64
}

// createYAxisBuckets creates evenly spaced Y-axis buckets
func createYAxisBuckets(minValue, maxValue uint64, numBuckets int) []yBucket {
	buckets := make([]yBucket, numBuckets)
	valueRange := float64(maxValue - minValue)
	bucketSize := valueRange / float64(numBuckets)

	for i := 0; i < numBuckets; i++ {
		buckets[i] = yBucket{
			min: minValue + uint64(float64(i)*bucketSize),
			max: minValue + uint64(float64(i+1)*bucketSize),
		}
	}

	// Ensure the last bucket includes the max value
	buckets[numBuckets-1].max = maxValue

	return buckets
}

// createTimeBuckets creates time bucket boundaries
func createTimeBuckets(start, end, step int64) []int64 {
	var buckets []int64
	for t := start; t <= end; t += step {
		buckets = append(buckets, t)
	}
	return buckets
}

// findTimeBucket finds the index of the time bucket for a given timestamp
func findTimeBucket(timestamp int64, buckets []int64) int {
	for i, bucket := range buckets {
		if timestamp <= bucket {
			return i
		}
	}
	// If timestamp is beyond all buckets, put it in the last bucket
	if len(buckets) > 0 {
		return len(buckets) - 1
	}
	return -1
}

// findValueBucket finds the index of the Y-axis bucket for a given value
func findValueBucket(value uint64, buckets []yBucket) int {
	for i, bucket := range buckets {
		if value >= bucket.min && value < bucket.max {
			return i
		}
	}
	// If value is at or beyond the last bucket max, put it in the last bucket
	if len(buckets) > 0 && value >= buckets[len(buckets)-1].min {
		return len(buckets) - 1
	}
	return -1
}
