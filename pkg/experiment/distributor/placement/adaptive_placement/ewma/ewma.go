package ewma

import (
	"math"
	"time"
)

// Rate is an exponentially weighted moving average event rate.
// The rate is always calculated per second and samples are
// summed up within the second duration.
type Rate struct {
	lifetime   float64
	last       int64
	cumulative float64
	ewma       float64
}

// New creates a new rate with a given lifetime.
//
// lifetime is the time required for the decaying quantity to
// reduced to 1⁄e ≈ 0.367879441 times its initial value.
func New(lifetime time.Duration) *Rate {
	return &Rate{lifetime: max(1, lifetime.Seconds())}
}

// NewHalfLife creates a new rate with a given half-life.
//
// halflife is the time required for the decaying quantity
// to fall to one half of its initial value.
func NewHalfLife(halflife time.Duration) *Rate {
	// https://en.wikipedia.org/wiki/Exponential_decay:
	//  halflife = ln(2)/λ = tau * ln(2)
	//  tau = 1/λ = halflife/ln(2)
	return &Rate{lifetime: max(1, halflife.Seconds()) / math.Ln2}
}

func (r *Rate) Value() float64   { return r.value(time.Now().UnixNano()) }
func (r *Rate) Update(v float64) { r.update(v, time.Now().UnixNano()) }

func (r *Rate) ValueAt(t int64) float64     { return r.value(t) }
func (r *Rate) UpdateAt(v float64, t int64) { r.update(v, t) }

func (r *Rate) LastUpdate() time.Time { return time.Unix(0, r.last) }

func (r *Rate) value(now int64) float64 {
	delta := float64(now - r.last)
	if delta < 0 {
		return 0
	}
	delta /= 1e9
	if delta >= 1 {
		// Correct the result for the time passed since the last update.
		// Over time, the delta increases and the decreased value (the
		// instant rate cumulative/delta) dominates in both ways:
		// directly and via the impact of the exponent. The value is
		// asymptotically approaching zero, if no updates are made.
		return ewma(r.ewma, r.cumulative/delta, delta, r.lifetime)
	}
	return r.ewma
}

// update updates the rate with a new value and timestamp in nanoseconds.
// It's assumed that time never goes backwards.
func (r *Rate) update(v float64, now int64) {
	delta := float64(now - r.last)
	if delta < 0 {
		return
	}
	delta /= 1e9
	if delta >= 1 {
		r.ewma = ewma(r.ewma, r.cumulative/delta, delta, r.lifetime)
		r.cumulative = 0
		r.last = now
	}
	r.cumulative += v
}

func ewma(old, value, delta, lifetime float64) float64 {
	alpha := 1 - math.Exp(-delta/lifetime)
	return alpha*value + (1-alpha)*old
}
