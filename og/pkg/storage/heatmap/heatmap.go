package heatmap

import "time"

type Heatmap struct {
	// Values matrix contain values that indicate count of value occurrences,
	// satisfying boundaries of the X and Y bins: [StartTime:EndTime) and (MinValue:MaxValue].
	// A value can be accessed via Values[x][y], where:
	//   0 <= x < TimeBuckets, and
	//   0 <= y < ValueBuckets.
	Values [][]uint64
	// TimeBuckets denotes number of ticks on the X-axis.
	TimeBuckets int64
	// ValueBuckets denotes number of ticks on the Y-axis.
	ValueBuckets int64
	// StartTime and EndTime indicate boundaries of the X axis.
	StartTime time.Time
	EndTime   time.Time
	// MinValue and MaxValue indicate boundaries of the Y axis.
	MinValue uint64
	MaxValue uint64
	// MinDepth and MaxDepth indicate boundaries of the Z axis: [MinDepth:MaxDepth].
	// MinDepth is the minimal non-zero value (count of value occurrences) that can
	// be found in Values.
	MinDepth uint64
	MaxDepth uint64
}

type HeatmapParams struct {
	// TimeBuckets denotes number of ticks on the X-axis.
	TimeBuckets int64
	// ValueBuckets denotes number of ticks on the Y-axis.
	ValueBuckets int64
	// StartTime and EndTime indicate boundaries of the X axis.
	StartTime time.Time
	EndTime   time.Time
	MinValue  uint64
	MaxValue  uint64
}

type HeatmapSketch struct {
	HeatmapParams
	Columns []HeatmapColumn
}

type HeatmapColumn struct {
	Values []uint64
	Counts []uint64
}
