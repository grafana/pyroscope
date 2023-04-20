package timeline

import (
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"

	v1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
)

// New generates a FlamebearerTimeline,
// backfilling any missing data with zeros
// It assumes:
// * Ordered
// * startMs is earlier than the first series value
// * endMs is after the last series value
func New(series *v1.Series, startMs int64, endMs int64, durationDeltaSec int64) *flamebearer.FlamebearerTimelineV1 {
	// ms to seconds
	startSec := startMs / 1000
	points := series.GetPoints()
	res := make([]uint64, len(points))

	if len(points) < 1 {
		return &flamebearer.FlamebearerTimelineV1{
			StartTime:     startSec,
			DurationDelta: durationDeltaSec,
			Samples:       backfill(startMs, endMs, durationDeltaSec),
		}
	}

	i := 0
	prev := points[0]
	for _, curr := range points {
		backfillNum := sizeToBackfill(prev.Timestamp, curr.Timestamp, durationDeltaSec)

		if backfillNum > 0 {
			// backfill + newValue
			bf := append(backfill(prev.Timestamp, curr.Timestamp, durationDeltaSec), uint64(curr.Value))

			// break the slice
			first := res[:i]
			second := res[i:]

			// add new backfilled items
			first = append(first, bf...)

			// concatenate the three slices to form the new slice
			res = append(first, second...)
			prev = curr
			i = i + int(backfillNum)
		} else {
			res[i] = uint64(curr.Value)
			prev = curr
			i = i + 1
		}
	}

	// Backfill with 0s for data that's not available
	firstAvailableData := points[0]
	lastAvailableData := points[len(points)-1]
	backFillHead := backfill(startMs, firstAvailableData.Timestamp, durationDeltaSec)
	backFillTail := backfill(lastAvailableData.Timestamp, endMs, durationDeltaSec)

	res = append(backFillHead, res...)
	res = append(res, backFillTail...)

	timeline := &flamebearer.FlamebearerTimelineV1{
		StartTime:     startSec,
		DurationDelta: durationDeltaSec,
		Samples:       res,
	}

	return timeline
}

// sizeToBackfill indicates how many items are needed to backfill
// if none are needed, a negative value is returned
func sizeToBackfill(startMs int64, endMs int64, stepSec int64) int64 {
	startSec := startMs / 1000
	endSec := endMs / 1000
	size := ((endSec - startSec) / stepSec) - 1
	return size
}

func backfill(startMs int64, endMs int64, stepSec int64) []uint64 {
	size := sizeToBackfill(startMs, endMs, stepSec)
	if size <= 0 {
		size = 0
	}
	return make([]uint64, size)
}
