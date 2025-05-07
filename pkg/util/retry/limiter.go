package retry

import (
	"context"
	"sync/atomic"

	"golang.org/x/time/rate"
)

// RateLimiter implements Throttler using golang.org/x/time/rate
// with the given rate (ops per second) and burst size.
type RateLimiter struct {
	limiter *rate.Limiter
}

func NewRateLimiter(ratePerSec float64, burst int) *RateLimiter {
	return &RateLimiter{limiter: rate.NewLimiter(rate.Limit(ratePerSec), burst)}
}

func (r *RateLimiter) Run(f func()) {
	_ = r.limiter.Wait(context.Background())
	f()
}

// Limiter limits the number of tasks executed.
// Once the limit is reached, no more runs will be done.
type Limiter struct {
	runs  int64
	limit int64
}

func NewLimiter(n int64) *Limiter { return &Limiter{limit: n} }

func (l *Limiter) Run(f func()) {
	if atomic.AddInt64(&l.runs, 1) <= l.limit {
		f()
	}
}
