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
	res := make([]uint64, 0, sizeToBackfill(startMs, endMs, durationDeltaSec))

	timeline := &flamebearer.FlamebearerTimelineV1{
		StartTime:     startSec,
		DurationDelta: durationDeltaSec,
		Samples:       []uint64{},
	}

	if len(points) < 1 {
		if n := sizeToBackfill(startMs, endMs, durationDeltaSec); n > 0 {
			timeline.Samples = res[:n]
		}
		return timeline
	}

	// Backfill before the first data point.
	firstAvailableData := points[0]
	if n := sizeToBackfill(startMs, firstAvailableData.Timestamp, durationDeltaSec); n > 0 {
		res = res[:len(res)+int(n)]
	}

	prev := points[0]
	for _, curr := range points {
		if n := sizeToBackfill(prev.Timestamp, curr.Timestamp, durationDeltaSec) - 1; n > 0 {
			// Insert backfill.
			res = res[:len(res)+int(n)]
		}

		res = append(res, uint64(curr.Value))
		prev = curr
	}

	// Backfill after the last data point.
	lastAvailableData := points[len(points)-1]
	if n := sizeToBackfill(lastAvailableData.Timestamp, endMs, durationDeltaSec) - 1; n > 0 {
		res = res[:len(res)+int(n)]
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
