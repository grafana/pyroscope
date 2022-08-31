package storage

import "time"

type Heatmap struct {
	// Values matrix contain values that indicate count of value occurrences,
	// satisfying boundaries of the X and Y bins: [StartTime:EndTime) and (MinValue:MaxValue].
	// A value can be accessed via Values[x][y], where:
	//   0 <= x < TimeBuckets, and
	//   0 <= y < ValueBuckets.
	Values [][]uint64
	// TimeBuckets denote number of bins on the X axis.
	// Length of Values array.
	TimeBuckets int
	// ValueBuckets denote number of bins on the Y axis.
	// Length of any item in the Values array.
	ValueBuckets int

	// StartTime and EndTime indicate boundaries of the X axis: [StartTime:EndTime).
	StartTime time.Time
	EndTime   time.Time

	// MinValue and MaxValue indicate boundaries of the Y axis: (MinValue:MaxValue].
	MinValue uint64
	MaxValue uint64

	// MinDepth and MaxDepth indicate boundaries of the Z axis: [MinDepth:MaxDepth].
	// MinDepth is the minimal non-zero value that can be found in Values.
	MinDepth uint64
	MaxDepth uint64
}
