package aggregator

import (
	"sync"
	"sync/atomic"
	"time"
)

// Aggregator aggregates values within
// a time window over a period of time.
type Aggregator[T any] struct {
	window int64
	period int64
	now    func() int64

	m          sync.RWMutex
	tracker    *tracker
	aggregates map[aggregationKey]*aggregate[T]

	close chan struct{}
	done  chan struct{}
	stats stats
}

type stats struct {
	activeAggregates atomic.Int64
	activeSeries     atomic.Uint64
	aggregated       atomic.Uint64
}

func NewAggregator[T any](window, period time.Duration) *Aggregator[T] {
	if window < period {
		window = period
	}
	return &Aggregator[T]{
		window:  window.Nanoseconds(),
		period:  period.Nanoseconds(),
		now:     timeNow,
		tracker: newTracker(8, 64),
		// NOTE(kolesnikovae): probably should be sharded as well.
		aggregates: make(map[aggregationKey]*aggregate[T], 256),
	}
}

func timeNow() int64 { return time.Now().UnixNano() }

func (a *Aggregator[T]) Start() {
	a.close = make(chan struct{})
	a.done = make(chan struct{})
	go func() {
		t := time.NewTicker(time.Duration(a.period))
		defer func() {
			t.Stop()
			close(a.done)
		}()
		for {
			select {
			case <-a.close:
				return
			case <-t.C:
				a.prune(a.now())
			}
		}
	}()
}

// Stop the aggregator. It does not wait for ongoing aggregations
// to complete as no aggregation requests expected during shutdown.
func (a *Aggregator[T]) Stop() {
	close(a.close)
	<-a.done
}

type AggregateFn[T any] func(T) T

type AggregationResult[T any] interface {
	// Wait blocks until the aggregation finishes.
	// The block duration never exceeds aggregation period.
	Wait() error
	// Value returns the aggregated value and indicates
	// whether the caller owns it.
	Value() (T, bool)
	// Close notifies all the contributors about the error
	// encountered. Owner of the aggregated result must
	// propagate any processing error happened with the value.
	Close(error)
}

func (a *Aggregator[T]) Aggregate(key uint64, timestamp int64, fn AggregateFn[T]) AggregationResult[T] {
	now := a.now()
	lastUpdated := a.tracker.update(key, now)
	if lastUpdated <= 0 || now-lastUpdated > a.period {
		// Event rate associated with the key is too low for aggregation.
		var empty T
		return resultWithoutAggregation[T]{fn(empty)}
	}
	k := a.aggregationKey(key, timestamp)
	a.m.Lock()
	x, ok := a.aggregates[k]
	if !ok {
		x = newAggregate[T](now + a.period)
		a.aggregates[k] = x
	}
	a.m.Unlock()
	x.m.Lock()
	if x.v == nil {
		a.stats.activeAggregates.Add(1)
		x.v = &aggregatedResult[T]{
			owner: make(chan struct{}, 1),
			done:  make(chan struct{}),
		}
		go a.waitResult(x)
	}
	a.stats.aggregated.Add(1)
	x.v.value = fn(x.v.value)
	r := x.v
	x.m.Unlock()
	return r
}

func (a *Aggregator[T]) aggregationKey(key uint64, timestamp int64) aggregationKey {
	return aggregationKey{
		timestamp: (timestamp / a.window) * a.window,
		key:       key,
	}
}

type aggregationKey struct {
	key       uint64
	timestamp int64
}

func (a *Aggregator[T]) waitResult(x *aggregate[T]) {
	// The value life-time is limited to the aggregation
	// window duration.
	<-time.After(time.Duration(a.period))
	// Invalidate the aggregate and release all the
	// contributors. They retain a copy of the aggregate
	// reference, therefore we can set it to nil.
	x.m.Lock()
	owner := x.v.owner
	x.v = nil
	x.m.Unlock()
	// Notify the owner: it must handle the aggregate
	// and close it, propagating any error occurred.
	owner <- struct{}{}
}

// prune removes stale aggregates that reached the deadline,
// and also removes keys that have not been updating since
// the beginning of the preceding aggregation period.
func (a *Aggregator[T]) prune(deadline int64) {
	a.m.Lock()
	for k, v := range a.aggregates {
		if v.deadline <= deadline {
			delete(a.aggregates, k)
			a.stats.activeAggregates.Add(-1)
		}
	}
	a.m.Unlock()
	a.tracker.prune(deadline - a.period)
	a.stats.activeSeries.Store(uint64(a.tracker.len()))
}

type aggregate[T any] struct {
	deadline int64

	m sync.Mutex
	v *aggregatedResult[T]
}

func newAggregate[T any](deadline int64) *aggregate[T] {
	return &aggregate[T]{deadline: deadline}
}

type aggregatedResult[T any] struct {
	handled atomic.Bool
	owner   chan struct{}
	value   T

	close sync.Once
	done  chan struct{}
	err   error
}

func (r *aggregatedResult[T]) Wait() error {
	select {
	case <-r.owner:
		// The first succeeded caller owns the aggregation
		// result and will be the first who calls Close,
		// therefore no error to be returned at this point.
		return nil
	case <-r.done:
		return r.err
	}
}

func (r *aggregatedResult[T]) Close(err error) {
	r.close.Do(func() {
		r.err = err
		close(r.done)
	})
}

func (r *aggregatedResult[T]) Value() (v T, ok bool) {
	return r.value, !r.handled.Swap(true)
}

type resultWithoutAggregation[T any] struct{ value T }

func (n resultWithoutAggregation[T]) Wait() error      { return nil }
func (n resultWithoutAggregation[T]) Value() (T, bool) { return n.value, true }
func (n resultWithoutAggregation[T]) Close(error)      {}

type tracker struct{ shards []*shard }

func newTracker(shards int, shardSize uint32) *tracker {
	t := tracker{shards: make([]*shard, shards)}
	for i := range t.shards {
		t.shards[i] = &shard{v: make(map[uint64]int64, shardSize)}
	}
	return &t
}

// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
func reduce(k, n uint64) uint64 { return (k * n) >> 32 }

func (t *tracker) shard(k uint64) *shard          { return t.shards[reduce(k, uint64(len(t.shards)))] }
func (t *tracker) update(k uint64, n int64) int64 { return t.shard(k).update(k, n) }

// prune removes keys with values less than n and
// reports the number of items remaining.
func (t *tracker) prune(n int64) {
	for _, x := range t.shards {
		x.prune(n)
	}
}

func (t *tracker) len() int {
	var n int
	for _, x := range t.shards {
		n += x.len()
	}
	return n
}

type shard struct {
	m sync.Mutex
	v map[uint64]int64
	s int
}

func (s *shard) update(k uint64, n int64) int64 {
	s.m.Lock()
	v := s.v[k]
	s.v[k] = n
	s.m.Unlock()
	return v
}

func (s *shard) prune(n int64) {
	s.m.Lock()
	s.s = len(s.v)
	for k, v := range s.v {
		if v <= n {
			delete(s.v, k)
			s.s--
		}
	}
	s.m.Unlock()
}

func (s *shard) len() int {
	s.m.Lock()
	v := s.s
	s.m.Unlock()
	return v
}
