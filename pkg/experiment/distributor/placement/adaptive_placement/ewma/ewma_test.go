package ewma

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_Rate_HalfLife(t *testing.T) {
	s := int64(0)
	r := NewHalfLife(time.Second * 10)

	// Expected rate 100.
	step := int64(1e9 / 10)    // 100ms
	for i := 0; i < 500; i++ { // 50s.
		r.update(10, s)
		if i == 100 { // 10s (half-life)
			// Half-life: 10s => 100 * 0.5 = 50.
			assert.InEpsilon(t, float64(50), r.value(s), 0.0001)
		}
		s += step
	}
	// Here and below: Value takes into account the time
	// since the last update, so we need to adjust it to
	// compensate the last iteration.
	assert.InEpsilon(t, 100, r.value(s-step), 0.05)

	// Rate decreases.
	step = int64(1e9 / 10)     // 100ms
	for i := 0; i < 500; i++ { // 50s.
		r.update(5, s)
		s += step
	}
	assert.InEpsilon(t, 50, r.value(s-step), 0.05)

	// Exactly 1s rate.
	step = int64(1e9)         // 1s
	for i := 0; i < 50; i++ { // 50s
		r.update(50, s)
		s += step
	}
	assert.InEpsilon(t, 50, r.value(s-step), 0.005)

	// Sub-second rate.
	step = int64(1e9 * 2)     // 2s
	for i := 0; i < 50; i++ { // 50s.
		r.update(1, s)
		if i == 5 { // 10s (half-life)
			// We expect that after expiration of the half-life interval,
			// the rate should be roughly 25: (~50 + 0.5) / 2 = ~25.
			// The numbers are not exact because r has state:
			// in the beginning it is slightly greater than 50.
			assert.InEpsilon(t, 25, int(r.value(s)), 0.0001)
		}
		s += step // Once per two seconds.
	}
	assert.InEpsilon(t, 0.5, r.value(s-step), 0.5)
}

func Test_Rate_HalfLife_Tail(t *testing.T) {
	// Expected rate 100.
	const (
		step   int64 = 1e9 / 10 // 100ms
		steps  int64 = 1200     // 120s.
		update int64 = 10

		rate     = update * int64(time.Second) / step
		halflife = time.Second * 10
	)

	r := NewHalfLife(halflife)
	var s int64
	for i := int64(0); i < steps; i++ {
		r.update(float64(update), s)
		s += step
		if i == int64(halflife)/step {
			// Just in case: check half-life value.
			assert.InEpsilon(t, float64(rate/2), r.value(s), 0.0001)
		}
	}

	assert.InEpsilon(t, float64(rate), r.value(s), 0.05)
	// Now we stop updating the rate and
	// expect that it will decay to zero.
	timespan := s + (steps * step)
	assert.Less(t, r.value(timespan), float64(1))
	// Half-life check: note that r is not exactly 100,
	// therefore we will have some error here.
	timespan = s + int64(halflife)
	assert.InEpsilon(t, r.value(timespan), r.value(s)/2, 0.05)
}

func Test_Rate_HalfLife_Complement(t *testing.T) {
	// The test examines the complementarity of two rates,
	// when the sum of the rates is constant.
	const (
		step   = int64(1e9 / 10) // 100ms
		steps  = 600             // 60s.
		update = 10

		rate     = update * int64(time.Second) / step
		halflife = time.Second * 10
	)

	s := int64(0)
	a := NewHalfLife(halflife)
	for i := 0; i < steps; i++ {
		a.update(update, s)
		s += step
	}
	assert.InEpsilon(t, float64(rate), a.value(s), 0.05)

	b := NewHalfLife(halflife)
	const interval = steps / 10
	for n := 0; n < steps; n += interval {
		for i := 0; i < interval; i++ {
			b.update(update, s)
			s += step
		}
		// The sum of the rates is expected to be constant.
		assert.InEpsilon(t, float64(rate), a.value(s)+b.value(s), 0.05)
	}
}

func Test_Rate_Lifetime(t *testing.T) {
	// Expected rate 100.
	const (
		step   int64 = 1e9 / 10 // 100ms
		steps  int64 = 100      // 10s.
		update int64 = 10

		rate = update * int64(time.Second) / step
		// lifetime/3 approximates SMA (error is ~5%).
		lifetime = time.Second * 10 / 3
	)

	r := New(lifetime)
	var s int64
	for i := int64(0); i < steps; i++ {
		r.update(float64(update), s)
		s += step
	}

	assert.InEpsilon(t, float64(rate), r.value(s), 0.05)
	// Now we stop updating the rate and expect
	// that it will decay to zero. Note that the
	// value decays more slowly: after 20 seconds
	// we still observe a non-zero value (~5%).
	for i := int64(0); i < 2*steps; i++ {
		s += step
	}
	assert.Less(t, r.value(s), float64(5))
}
