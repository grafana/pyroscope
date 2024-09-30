package ewma

import (
	"math"
	"time"
)

// Rate is an exponentially weighted moving average rate tracker
// The rate is always calculated per second and samples are
// summed up within the second duration.
type Rate struct {
	window     float64
	last       int64
	cumulative float64
	ewma       float64
}

func New(window time.Duration) *Rate { return &Rate{window: max(1, window.Seconds())} }

func (r *Rate) Value() float64 { return r.ewma }

func (r *Rate) Update(v float64) { r.Add(v, time.Now().UnixNano()) }

func (r *Rate) LastUpdate() time.Time { return time.Unix(0, r.last) }

// Add updates the rate with a new value and timestamp in nanoseconds.
// It's assumed that time never goes backwards.
func (r *Rate) Add(v float64, now int64) {
	delta := float64(now - r.last)
	if delta < 0 {
		return
	}
	delta /= 1e9
	if delta >= 1 {
		r.ewma = ewma(r.ewma, r.cumulative/delta, delta, r.window)
		r.cumulative = 0
		r.last = now
	}
	r.cumulative += v
}

func ewma(old, value, delta, window float64) float64 {
	alpha := 1 - math.Exp(-delta/window)
	return alpha*value + (1-alpha)*old
}
