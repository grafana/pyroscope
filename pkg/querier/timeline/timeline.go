package timeline

import (
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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

	timeline := &flamebearer.FlamebearerTimelineV1{
		StartTime:     startSec,
		DurationDelta: durationDeltaSec,
		Samples:       []uint64{},
	}

	if len(points) < 1 {
		if n := sizeToBackfill(startMs, endMs, durationDeltaSec); n > 0 {
			timeline.Samples = make([]uint64, n)
		}
		return timeline
	}

	i := 0
	prev := points[0]
	for _, curr := range points {
		res[i] = uint64(curr.Value)

		backfillNum := sizeToBackfill(prev.Timestamp, curr.Timestamp, durationDeltaSec)
		if backfillNum > 0 {
			// Subtract 1 to account for the current value already being added
			// to the result slice.
			backfillNum--

			// Insert backfill.
			bf := make([]uint64, backfillNum)
			res = append(res[:i], append(bf, res[i:]...)...)

		} else {
			backfillNum = 0
		}

		i += int(backfillNum) + 1
		prev = curr
	}

	// Backfill with 0s for data that's not available
	firstAvailableData := points[0]
	lastAvailableData := points[len(points)-1]

	if n := sizeToBackfill(startMs, firstAvailableData.Timestamp, durationDeltaSec); n > 0 {
		bf := make([]uint64, n)
		res = append(bf, res...)
	}

	if n := sizeToBackfill(lastAvailableData.Timestamp, endMs, durationDeltaSec) - 1; n > 0 {
		bf := make([]uint64, n)
		res = append(res, bf...)
	}

	timeline.Samples = res
	return timeline
}

// sizeToBackfill indicates how many items are needed to backfill
// if none are needed, a negative value is returned
func sizeToBackfill(startMs int64, endMs int64, stepSec int64) int64 {
	startSec := startMs / 1000
	endSec := endMs / 1000
	size := (endSec - startSec) / stepSec
	return size
}
