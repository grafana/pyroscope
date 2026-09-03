package inflight

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
)

// Limiter tracks the total size of the requests an instance currently holds
// in memory, and rejects new reservations once the configured limit is
// reached. A zero or negative limit disables rejection; the accounting is
// always maintained, so that the limit can be sized before it is enabled.
//
// The limiter is not a semaphore: reservations are never queued. A request
// that does not fit is rejected, and the caller is expected to retry.
type Limiter struct {
	limit         int64
	bytes         atomic.Int64
	highWatermark prometheus.Observer
}

// NewLimiter returns a limiter bounded by limit. Every reservation reports the
// resulting total to highWatermark, which may be nil. The total is only
// meaningful at the moment a reservation is taken, so it is sampled here
// rather than read at scrape time, when the peak has usually passed.
func NewLimiter(limit int64, highWatermark prometheus.Observer) *Limiter {
	return &Limiter{limit: limit, highWatermark: highWatermark}
}

// Bytes returns the number of bytes currently reserved.
func (l *Limiter) Bytes() int64 { return l.bytes.Load() }

// Reserve accounts for size bytes and returns the reservation. The caller
// owns a single reference to it and must release it exactly once.
//
// The boolean return reports whether the reservation fits within the limit.
// The reservation is valid and must be released even if it does not fit:
// this way the counter reflects the memory the caller actually holds, and
// the rejection decision is left to the caller.
func (l *Limiter) Reserve(size int64) (*Reservation, bool) {
	r := &Reservation{limiter: l, size: size}
	r.refs.Store(1)
	total := l.bytes.Add(size)
	if l.highWatermark != nil {
		l.highWatermark.Observe(float64(total))
	}
	return r, l.limit <= 0 || total <= l.limit
}

// Reservation is a reference-counted claim on the limiter capacity. The
// claim is returned once the last reference is released, which allows the
// accounting to span asynchronous continuations of a request.
type Reservation struct {
	limiter *Limiter
	size    int64
	refs    atomic.Int32
}

// Retain acquires an additional reference. It reports false if the
// reservation has already been released in full, in which case no reference
// is acquired and Release must not be called.
func (r *Reservation) Retain() bool {
	if r == nil {
		return false
	}
	for {
		refs := r.refs.Load()
		if refs <= 0 {
			return false
		}
		if r.refs.CompareAndSwap(refs, refs+1) {
			return true
		}
	}
}

// Release drops a reference. The reserved bytes are returned to the limiter
// when the last reference is dropped.
func (r *Reservation) Release() {
	if r == nil {
		return
	}
	if r.refs.Dec() == 0 {
		r.limiter.bytes.Sub(r.size)
	}
}

type contextKey struct{}

// NewContext returns a context carrying the reservation. A nil reservation
// detaches the parent context from any reservation it carries, which is what
// work that outlives the originating request must do.
func NewContext(ctx context.Context, r *Reservation) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}

// FromContext returns the reservation carried by ctx, or nil.
func FromContext(ctx context.Context) *Reservation {
	r, _ := ctx.Value(contextKey{}).(*Reservation)
	return r
}

// Detach retains the reservation carried by ctx on behalf of work that
// continues after the caller returns, and returns the matching release
// function. It is always safe to call, and the returned function is always
// safe to call exactly once.
func Detach(ctx context.Context) func() {
	r := FromContext(ctx)
	if !r.Retain() {
		return func() {}
	}
	return r.Release
}
