// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/time.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package util

import (
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/grafana/dskit/httpgrpc"
	"github.com/prometheus/common/model"
)

func TimeToMillis(t time.Time) int64 {
	return t.Unix()*1000 + int64(t.Nanosecond())/int64(time.Millisecond)
}

// TimeFromMillis is a helper to turn milliseconds -> time.Time
func TimeFromMillis(ms int64) time.Time {
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond)).UTC()
}

// FormatTimeMillis returns a human readable version of the input time (in milliseconds).
func FormatTimeMillis(ms int64) string {
	return TimeFromMillis(ms).String()
}

// FormatTimeModel returns a human readable version of the input time.
func FormatTimeModel(t model.Time) string {
	return TimeFromMillis(int64(t)).String()
}

// ParseTime parses the string into an int64, milliseconds since epoch.
func ParseTime(s string) (int64, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		ns = math.Round(ns*1000) / 1000
		tm := time.Unix(int64(s), int64(ns*float64(time.Second)))
		return TimeToMillis(tm), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return TimeToMillis(t), nil
	}
	return 0, httpgrpc.Errorf(http.StatusBadRequest, "cannot parse %q to a valid timestamp", s)
}

// DurationWithJitter returns random duration from "input - input*variance" to "input + input*variance" interval.
func DurationWithJitter(input time.Duration, variancePerc float64) time.Duration {
	// No duration? No jitter.
	if input == 0 {
		return 0
	}

	variance := int64(float64(input) * variancePerc)
	jitter := rand.Int63n(variance*2) - variance

	return input + time.Duration(jitter)
}

// DurationWithPositiveJitter returns random duration from "input" to "input + input*variance" interval.
func DurationWithPositiveJitter(input time.Duration, variancePerc float64) time.Duration {
	// No duration? No jitter.
	if input == 0 {
		return 0
	}

	variance := int64(float64(input) * variancePerc)
	jitter := rand.Int63n(variance)

	return input + time.Duration(jitter)
}

// DurationWithNegativeJitter returns random duration from "input - input*variance" to "input" interval.
func DurationWithNegativeJitter(input time.Duration, variancePerc float64) time.Duration {
	// No duration? No jitter.
	if input == 0 {
		return 0
	}

	variance := int64(float64(input) * variancePerc)
	jitter := rand.Int63n(variance)

	return input - time.Duration(jitter)
}

// NewDisableableTicker essentially wraps NewTicker but allows the ticker to be disabled by passing
// zero duration as the interval. Returns a function for stopping the ticker, and the ticker channel.
func NewDisableableTicker(interval time.Duration) (func(), <-chan time.Time) {
	if interval == 0 {
		return func() {}, nil
	}

	tick := time.NewTicker(interval)
	return func() { tick.Stop() }, tick.C
}

type TimeRange struct {
	Start      time.Time
	End        time.Time
	Resolution time.Duration
}

// SplitTimeRangeByResolution splits the given time range into the
// minimal number of non-overlapping sub-ranges aligned with resolutions.
// All ranges have inclusive start and end; one millisecond step.
func SplitTimeRangeByResolution(start, end time.Time, resolutions []time.Duration, fn func(TimeRange)) {
	if len(resolutions) == 0 {
		fn(TimeRange{Start: start, End: end})
		return
	}

	// Sorting resolutions in ascending order.
	sort.Slice(resolutions, func(i, j int) bool {
		return resolutions[i] > resolutions[j]
	})

	// Time ranges are inclusive on both ends. In order to simplify calculation
	// of resolution alignment, we add a millisecond to the end time.
	// Added millisecond is subtracted from the final ranges.
	end = end.Add(time.Millisecond)
	var (
		c = start       // Current range start position.
		r time.Duration // Current resolution.
	)

	for c.Before(end) {
		var d time.Duration = -1
		// Find the lowest resolution aligned with the current position.
		for _, res := range resolutions {
			if c.UnixNano()%res.Nanoseconds() == 0 && !c.Add(res).After(end) {
				d = res
				break
			}
		}
		res := d
		if d < 0 {
			// No suitable resolution found: add distance
			// to the next closest aligned boundary.
			l := resolutions[len(resolutions)-1]
			d = l - time.Duration(c.UnixNano()%l.Nanoseconds())
		}
		if end.Before(c.Add(d)) {
			d = end.Sub(c)
		}
		// If the resolution has changed, emit a new range.
		if r != res && c.After(start) {
			fn(TimeRange{Start: start, End: c.Add(-time.Millisecond), Resolution: r})
			start = c
		}
		c = c.Add(d)
		r = res
	}

	if start != c {
		fn(TimeRange{Start: start, End: c.Add(-time.Millisecond), Resolution: r})
	}
}
