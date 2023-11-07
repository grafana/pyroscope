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
	aggregates map[aggregationKey]*AggregationResult[T]

	close chan struct{}
	done  chan struct{}
	stats stats
}

type stats struct {
	activeAggregates atomic.Int64
	activeSeries     atomic.Uint64
	aggregated       atomic.Uint64
	errors           atomic.Uint64
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
		aggregates: make(map[aggregationKey]*AggregationResult[T], 256),
		close:      make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func timeNow() int64 { return time.Now().UnixNano() }

func (a *Aggregator[T]) Start() {
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
}

// Stop the aggregator. It does not wait for ongoing aggregations
// to complete as no aggregation requests expected during shutdown.
func (a *Aggregator[T]) Stop() {
	close(a.close)
	<-a.done
}

type AggregateFn[T any] func(T) (T, error)

func (a *Aggregator[T]) Aggregate(key uint64, timestamp int64, fn AggregateFn[T]) (*AggregationResult[T], bool, error) {
	// Return early if the event rate is too low for aggregation.
	now := a.now()
	lastUpdated := a.tracker.update(key, now)
	delta := now - lastUpdated // Negative delta is possible.
	// Distance between two updates is longer than the aggregation period.
	lowRate := 0 < delta && delta > a.period
	if lastUpdated == 0 || lowRate {
		return nil, false, nil
	}
	k := a.aggregationKey(key, timestamp)
	a.m.Lock()
	x, ok := a.aggregates[k]
	if !ok {
		a.stats.activeAggregates.Add(1)
		x = &AggregationResult[T]{
			key:   k,
			owner: make(chan struct{}, 1),
			done:  make(chan struct{}),
		}
		a.aggregates[k] = x
		go a.waitResult(x)
	}
	x.wg.Add(1)
	defer x.wg.Done()
	a.m.Unlock()
	select {
	default:
	case <-x.done:
		// Aggregation has failed.
		return x, true, x.err
	}
	var err error
	x.m.Lock()
	x.value, err = fn(x.value)
	x.m.Unlock()
	if err != nil {
		a.stats.errors.Add(1)
		x.Close(err)
	} else {
		a.stats.aggregated.Add(1)
	}
	return x, true, err
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

func (a *Aggregator[T]) waitResult(x *AggregationResult[T]) {
	// The value life-time is limited to the aggregation
	// window duration.
	var failed bool
	select {
	case <-time.After(time.Duration(a.period)):
	case <-x.done:
		failed = true
	}
	a.m.Lock()
	delete(a.aggregates, x.key)
	a.m.Unlock()
	a.stats.activeAggregates.Add(-1)
	if !failed {
		// Wait for ongoing aggregations to finish.
		x.wg.Wait()
		// Notify the owner: it must handle the aggregate
		// and close it, propagating any error occurred.
		x.owner <- struct{}{}
	}
}

// prune removes keys that have not been updating since
// the beginning of the preceding aggregation period.
func (a *Aggregator[T]) prune(deadline int64) {
	a.tracker.prune(deadline - a.period)
	a.stats.activeSeries.Store(uint64(a.tracker.len()))
}

type AggregationResult[T any] struct {
	key     aggregationKey
	handled atomic.Bool
	owner   chan struct{}
	m       sync.Mutex
	value   T

	wg    sync.WaitGroup
	close sync.Once
	done  chan struct{}
	err   error
}

// Wait blocks until the aggregation finishes.
// The block duration never exceeds aggregation period.
func (r *AggregationResult[T]) Wait() error {
	select {
	case <-r.owner:
	case <-r.done:
	}
	return r.err
}

// Close notifies all the contributors about the error
// encountered. Owner of the aggregated result must
// propagate any processing error happened with the value.
func (r *AggregationResult[T]) Close(err error) {
	r.close.Do(func() {
		r.err = err
		close(r.done)
	})
}

// Value returns the aggregated value and indicates
// whether the caller owns it.
func (r *AggregationResult[T]) Value() (v T, ok bool) {
	return r.value, !r.handled.Swap(true)
}

// Handler returns a handler of the aggregated result.
// The handler is nil, if it has already been acquired.
// The returned function is synchronous and blocks for
// up to the aggregation period duration.
func (r *AggregationResult[T]) Handler() func() (T, error) {
	if !r.handled.Swap(true) {
		return r.handle
	}
	return nil
}

func (r *AggregationResult[T]) handle() (v T, err error) {
	defer r.Close(err)
	if err = r.Wait(); err != nil {
		return v, err
	}
	return r.value, r.err
}

type tracker struct{ shards []*shard }

func newTracker(shards int, shardSize uint32) *tracker {
	t := tracker{shards: make([]*shard, shards)}
	for i := range t.shards {
		t.shards[i] = &shard{v: make(map[uint64]int64, shardSize)}
	}
	return &t
}

func (t *tracker) shard(k uint64) *shard          { return t.shards[k%uint64(len(t.shards))] }
func (t *tracker) update(k uint64, n int64) int64 { return t.shard(k).update(k, n) }

// prune removes keys with values less than n.
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
