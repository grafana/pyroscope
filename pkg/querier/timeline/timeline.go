package timeline

import (
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// New generates a FlamebearerTimeline, backfilling any missing data with zeros.
// If startMs or endMs are in the middle of its durationDeltaSec bucket, they
// will be snapped to the beginning of that bucket.
//
// It assumes:
// * Ordered
// * Series timestamps are within [startMs, endMs)
func New(series *v1.Series, startMs int64, endMs int64, durationDeltaSec int64) *flamebearer.FlamebearerTimelineV1 {
	// Snap startMs and endMs to bucket boundaries.
	durationDeltaMs := durationDeltaSec * 1000
	startMs = (startMs / durationDeltaMs) * durationDeltaMs
	endMs = (endMs / durationDeltaMs) * durationDeltaMs

	// ms to seconds
	startSec := startMs / 1000
	points := series.GetPoints()

	timeline := &flamebearer.FlamebearerTimelineV1{
		StartTime:     startSec,
		DurationDelta: durationDeltaSec,
		Samples:       []uint64{},
	}
	if startMs >= endMs {
		return timeline
	}
	samples := make([]uint64, 0, sizeToBackfill(startMs, endMs, durationDeltaSec))

	// Ensure the points slice has only values in [startMs, endMs).
	points = boundPointsToWindow(points, startMs, endMs)
	if len(points) < 1 {
		if n := sizeToBackfill(startMs, endMs, durationDeltaSec); n > 0 {
			timeline.Samples = samples[:n]
		}
		return timeline
	}

	// Backfill before the first data point.
	firstAvailableData := points[0]
	if n := sizeToBackfill(startMs, firstAvailableData.Timestamp, durationDeltaSec); n > 0 {
		samples = samples[:len(samples)+int(n)]
	}

	prev := points[0]
	for _, curr := range points {
		if n := sizeToBackfill(prev.Timestamp, curr.Timestamp, durationDeltaSec) - 1; n > 0 {
			// Insert backfill.
			samples = samples[:len(samples)+int(n)]
		}

		samples = append(samples, uint64(curr.Value))
		prev = curr
	}

	// Backfill to the end of the samples window.
	samples = samples[:cap(samples)]

	timeline.Samples = samples
	return timeline
}

// sizeToBackfill indicates how many items are needed to backfill in the range
// [startMs, endMs).
func sizeToBackfill(startMs int64, endMs int64, stepSec int64) int64 {
	startSec := startMs / 1000
	endSec := endMs / 1000
	size := (endSec - startSec) / stepSec
	return size
}

// boundPointsToWindow will return a slice of points such that all the points
// are constrained within [startMs, endMs).
func boundPointsToWindow(points []*v1.Point, startMs int64, endMs int64) []*v1.Point {
	start := 0
	for ; start < len(points); start++ {
		if points[start].Timestamp >= startMs {
			break
		}
	}
	points = points[start:]

	end := len(points) - 1
	for ; end >= 0; end-- {
		if points[end].Timestamp < endMs {
			break
		}
	}
	points = points[:end+1]

	return points
}
