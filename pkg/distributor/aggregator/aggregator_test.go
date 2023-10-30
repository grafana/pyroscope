package aggregator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_Aggregation(t *testing.T) {
	w := time.Second * 15
	d := time.Millisecond * 10

	fn := func(i int) int {
		return i + 1
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	r1 := a.Aggregate(0, 0, fn)
	r2 := a.Aggregate(0, 1, fn)
	r3 := a.Aggregate(0, 2, fn)

	assert.NoError(t, r1.Wait())
	v, ok := r1.Value()
	// r1 owns the value as it was not aggregated.
	assert.True(t, ok)
	assert.Equal(t, 1, v)
	r1.Close(nil)

	assert.NoError(t, r2.Wait())
	v, ok = r2.Value()
	assert.Equal(t, 2, v)
	assert.True(t, ok)
	r2.Close(nil)

	assert.NoError(t, r3.Wait())
	v, ok = r3.Value()
	assert.False(t, ok)
	r3.Close(nil)
}

func Test_Aggregation_Concurrency(t *testing.T) {
	const (
		N = 10
		M = 300
		w = time.Millisecond * 100
		d = time.Millisecond
	)

	var (
		wg  sync.WaitGroup
		sum int64
		cnt int64
	)
	fn := func(i int) int {
		return i + 1
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			for j := int64(0); j < M; j++ {
				r := a.Aggregate(0, j, fn)
				assert.NoError(t, r.Wait())
				v, ok := r.Value()
				if ok {
					atomic.AddInt64(&sum, int64(v))
					atomic.AddInt64(&cnt, 1)
				}
				r.Close(nil)
				if j%(M/30) == 0 {
					a.prune(j)
				}
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(N*M), sum)
	// The number of aggregation is not deterministic.
	// However, we can assess if they happen at all.
	assert.Less(t, cnt, int64(M*N))
}

func Test_Aggregation_Error_Propagation(t *testing.T) {
	w := time.Second * 15
	d := time.Millisecond * 10

	fn := func(i int) int {
		return i + 1
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	r1 := a.Aggregate(0, 0, fn)
	r2 := a.Aggregate(0, 1, fn)
	r3 := a.Aggregate(0, 2, fn)

	assert.NoError(t, r1.Wait())
	v, ok := r1.Value()
	// r1 owns the value as it was not aggregated.
	assert.True(t, ok)
	assert.Equal(t, 1, v)
	r1.Close(nil)

	assert.NoError(t, r2.Wait())
	v, ok = r2.Value()
	assert.Equal(t, 2, v)
	assert.True(t, ok)
	r2.Close(context.Canceled)

	assert.ErrorIs(t, r3.Wait(), context.Canceled)
}

func Test_Aggregation_Pruning(t *testing.T) {
	w := time.Second * 15
	d := time.Millisecond * 10

	fn := func(i int) int {
		return i + 1
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	r1 := a.Aggregate(0, 0, fn)
	// In order to create aggregate we need at least
	// two requests within the aggregation window.
	_ = a.Aggregate(0, 1, fn)
	assert.NoError(t, r1.Wait())
	v, ok := r1.Value()
	// r1 owns the value as it was not aggregated.
	assert.True(t, ok)
	assert.Equal(t, 1, v)
	r1.Close(nil)
	// Evict stale aggregates and keys.
	a.prune(int64(d) + a.now())
	assert.Zero(t, len(a.aggregates))
	assert.Zero(t, a.tracker.update(0, 1))
}
