package heatmap

import (
	"encoding/binary"
	"encoding/hex"
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
// Each input series produces a separate output series with labels preserved
// Y-axis buckets are calculated based on global min/max across all series
func RangeHeatmap(
	reports []*queryv1.HeatmapReport,
	start, end, step int64,
	groupBy []string,
	exemplarType typesv1.ExemplarType,
) []*typesv1.HeatmapSeries {
	if len(reports) == 0 {
		return nil
	}

	// First pass: collect all series with their labels and find global min/max
	type seriesData struct {
		series *queryv1.HeatmapSeries
		labels []*typesv1.LabelPair
	}
	var allSeries []seriesData
	var globalMinValue, globalMaxValue int64 = math.MaxInt64, 0

	for _, report := range reports {
		if report == nil {
			continue
		}

		for _, series := range report.HeatmapSeries {
			if series == nil || len(series.Points) == 0 {
				continue
			}

			// Resolve labels from attribute refs
			labels := resolveAttributeRefs(series.AttributeRefs, report.AttributeTable)

			// Track this series
			allSeries = append(allSeries, seriesData{
				series: series,
				labels: labels,
			})

			// Update global min/max
			for _, point := range series.Points {
				if point == nil {
					continue
				}
				if point.Value < globalMinValue {
					globalMinValue = point.Value
				}
				if point.Value > globalMaxValue {
					globalMaxValue = point.Value
				}
			}
		}
	}

	if len(allSeries) == 0 || globalMinValue == math.MaxInt64 {
		return nil
	}

	// Handle edge case where all values are the same
	if globalMinValue == globalMaxValue {
		globalMaxValue = globalMinValue + 1
	}

	// Create Y-axis bucket boundaries based on global min/max
	yBuckets := createYAxisBuckets(globalMinValue, globalMaxValue, DefaultYAxisBuckets)

	// Check if exemplars should be included
	includeExemplars := exemplarType != typesv1.ExemplarType_EXEMPLAR_TYPE_NONE &&
		exemplarType != typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED

	// Get attribute table from first report for exemplar conversion
	var attributeTable *queryv1.AttributeTable
	if includeExemplars {
		for _, report := range reports {
			if report != nil && report.AttributeTable != nil {
				attributeTable = report.AttributeTable
				break
			}
		}
	}

	// Second pass: process each series with the shared Y-axis buckets
	var result []*typesv1.HeatmapSeries

	for _, sd := range allSeries {
		// Create a map to track counts per (time, y-bucket)
		type cellKey struct {
			ts   int64
			yIdx int
		}
		cellCounts := make(map[cellKey]int32)
		cellBestPoint := make(map[cellKey]*queryv1.HeatmapPoint) // Track best exemplar per cell

		// First pass: count and track best exemplar per cell
		for _, point := range sd.series.Points {
			if point == nil {
				continue
			}

			// normalize timestamp
			ts := normalizeTimestamp(point.Timestamp, start, end, step)

			// Find value bucket
			yIdx := findValueBucket(point.Value, yBuckets)
			if yIdx < 0 {
				continue
			}

			cell := cellKey{ts, yIdx}
			cellCounts[cell]++

			// Track highest-value point for this cell (for exemplar)
			if includeExemplars {
				if existing := cellBestPoint[cell]; existing == nil || point.Value > existing.Value {
					cellBestPoint[cell] = point
				}
			}
		}

		// Second pass: build slots with counts and exemplars
		slotsMap := make(map[int64]*typesv1.HeatmapSlot)
		for cell, count := range cellCounts {
			timestamp := cell.ts
			slot, ok := slotsMap[timestamp]
			if !ok {
				slot = &typesv1.HeatmapSlot{
					Timestamp: timestamp,
					YMin:      make([]float64, len(yBuckets)),
					Counts:    make([]int32, len(yBuckets)),
				}
				if includeExemplars {
					slot.Exemplars = make([]*typesv1.Exemplar, 0, len(yBuckets))
				}
				// Initialize Y bucket minimums
				for i, bucket := range yBuckets {
					slot.YMin[i] = float64(bucket.min)
				}
				slotsMap[timestamp] = slot
			}
			slot.Counts[cell.yIdx] = count

			// Attach exemplar for this Y-bucket
			if includeExemplars {
				if bestPoint := cellBestPoint[cell]; bestPoint != nil {
					e := pointToExemplar(bestPoint, attributeTable, groupBy)
					if e != nil {
						slot.Exemplars = append(slot.Exemplars, e)
					}
				}
			}
		}

		// Convert map to sorted slice
		var slots []*typesv1.HeatmapSlot
		for _, slot := range slotsMap {
			slots = append(slots, slot)
		}
		sort.Slice(slots, func(i, j int) bool {
			return slots[i].Timestamp < slots[j].Timestamp
		})

		// Add this series to the result with its labels preserved
		result = append(result, &typesv1.HeatmapSeries{
			Labels: sd.labels,
			Slots:  slots,
		})
	}

	return result
}

// resolveAttributeRefs converts attribute references to actual label pairs
// AttributeTable has parallel arrays: Keys[i] and Values[i] form a label pair
func resolveAttributeRefs(refs []int64, table *queryv1.AttributeTable) []*typesv1.LabelPair {
	if table == nil || len(refs) == 0 {
		return nil
	}

	labels := make([]*typesv1.LabelPair, 0, len(refs))
	for _, ref := range refs {
		// Validate index
		if ref < 0 || ref >= int64(len(table.Keys)) || ref >= int64(len(table.Values)) {
			continue
		}

		labels = append(labels, &typesv1.LabelPair{
			Name:  table.Keys[ref],
			Value: table.Values[ref],
		})
	}

	return labels
}

// yBucket represents a Y-axis bucket
type yBucket struct {
	min int64
	max int64
}

// createYAxisBuckets creates evenly spaced Y-axis buckets
func createYAxisBuckets(minValue, maxValue int64, numBuckets int) []yBucket {
	buckets := make([]yBucket, numBuckets)
	valueRange := float64(maxValue - minValue)
	bucketSize := valueRange / float64(numBuckets)

	for i := 0; i < numBuckets; i++ {
		buckets[i] = yBucket{
			min: minValue + int64(float64(i)*bucketSize),
			max: minValue + int64(float64(i+1)*bucketSize),
		}
	}

	// Ensure the last bucket includes the max value
	buckets[numBuckets-1].max = maxValue

	return buckets
}

func roundUpTimestamp(timestamp, start, step int64) int64 {
	startMod := start % step
	timestamp -= startMod
	if timestamp%step != 0 {
		timestamp = ((timestamp / step) + 1) * step
	}
	return timestamp + startMod
}

// normalizeTimestamp calculates the timestamp (xMax) of the bucket the timestamp falls into.
// The buckets range from:
// (start-step, start], (start, start+step], (start+step, start+2*step], ...
func normalizeTimestamp(timestamp, start, end, step int64) int64 {
	timestamp = roundUpTimestamp(timestamp, start, step)
	end = roundUpTimestamp(end, start, step)

	if timestamp < start {
		return start
	}

	if timestamp > end {
		return end
	}

	return timestamp
}

// findValueBucket finds the index of the Y-axis bucket for a given value
func findValueBucket(value int64, buckets []yBucket) int {
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

// filterExemplarLabels removes group_by labels from exemplar labels.
func filterExemplarLabels(labels []*typesv1.LabelPair, groupBy []string) []*typesv1.LabelPair {
	if len(groupBy) == 0 {
		return labels
	}

	groupBySet := make(map[string]struct{}, len(groupBy))
	for _, name := range groupBy {
		groupBySet[name] = struct{}{}
	}

	filtered := make([]*typesv1.LabelPair, 0, len(labels))
	for _, label := range labels {
		if _, isGroupBy := groupBySet[label.Name]; !isGroupBy {
			filtered = append(filtered, label)
		}
	}
	return filtered
}

// pointToExemplar converts a single HeatmapPoint to an Exemplar.
func pointToExemplar(
	point *queryv1.HeatmapPoint,
	table *queryv1.AttributeTable,
	groupBy []string,
) *typesv1.Exemplar {
	if point == nil || table == nil {
		return nil
	}

	// Resolve labels from attribute refs
	pointLabels := resolveAttributeRefs(point.AttributeRefs, table)

	// Filter out group_by labels
	filteredLabels := filterExemplarLabels(pointLabels, groupBy)

	// Resolve profile ID from attribute table
	profileID := ""
	if point.ProfileId >= 0 && point.ProfileId < int64(len(table.Values)) {
		profileID = table.Values[point.ProfileId]
	}

	// Convert span ID to hex string (little-endian, to match NewSpanSelector)
	spanIDStr := ""
	if point.SpanId != 0 {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, point.SpanId)
		spanIDStr = hex.EncodeToString(b)
	}

	// Don't create an exemplar if it has no identifying information
	// An exemplar needs at least one of: profile ID, span ID, or labels
	if profileID == "" && spanIDStr == "" && len(filteredLabels) == 0 {
		return nil
	}

	return &typesv1.Exemplar{
		Timestamp: point.Timestamp,
		ProfileId: profileID,
		SpanId:    spanIDStr,
		Value:     point.Value,
		Labels:    filteredLabels,
	}
}
