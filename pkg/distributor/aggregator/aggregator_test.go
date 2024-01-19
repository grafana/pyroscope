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

	fn := func(i int) (int, error) {
		return i + 1, nil
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	_, ok, err := a.Aggregate(0, 0, fn)
	assert.NoError(t, err)
	assert.False(t, ok)
	r2, ok, err := a.Aggregate(0, 1, fn)
	assert.NoError(t, err)
	assert.True(t, ok)
	r3, ok, err := a.Aggregate(0, 2, fn)
	assert.NoError(t, err)
	assert.True(t, ok)

	assert.NoError(t, r2.Wait())
	v, ok := r2.Value()
	assert.Equal(t, 2, v)
	assert.True(t, ok)
	r2.Close(nil)

	assert.NoError(t, r3.Wait())
	_, ok = r3.Value()
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
	fn := func(i int) (int, error) {
		return i + 1, nil
	}

	a := NewAggregator[int](w, d)
	var start atomic.Int64
	a.now = func() int64 {
		return start.Add(10)
	}

	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			for j := int64(0); j < M; j++ {
				r, ok, err := a.Aggregate(0, j, fn)
				if !ok {
					continue
				}
				assert.NoError(t, err)
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
	assert.Equal(t, int64(N*M)-1, sum)
	// The number of aggregation is not deterministic.
	// However, we can assess if they happen at all.
	assert.Less(t, cnt, int64(M*N))
}

func Test_Aggregation_Error(t *testing.T) {
	w := time.Second * 15
	d := time.Millisecond * 10

	fn := func(i int) (int, error) {
		return i + 1, nil
	}

	t.Run("Close with error", func(t *testing.T) {
		a := NewAggregator[int](w, d)
		var start int64
		a.now = func() int64 {
			start += 10
			return start
		}

		_, ok, err := a.Aggregate(0, 0, fn)
		assert.NoError(t, err)
		assert.False(t, ok)

		r2, ok, err := a.Aggregate(0, 1, fn)
		assert.NoError(t, err)
		assert.True(t, ok)

		r3, ok, err := a.Aggregate(0, 2, fn)
		assert.NoError(t, err)
		assert.True(t, ok)

		assert.NoError(t, r2.Wait())
		v, ok := r2.Value()
		assert.Equal(t, 2, v)
		assert.True(t, ok)
		r2.Close(context.Canceled)

		assert.ErrorIs(t, r3.Wait(), context.Canceled)
	})

	t.Run("First aggregation failed", func(t *testing.T) {
		a := NewAggregator[int](w, d)
		var start int64
		a.now = func() int64 {
			start += 10
			return start
		}

		_, ok, err := a.Aggregate(0, 0, fn)
		assert.NoError(t, err)
		assert.False(t, ok)

		r3, _, err := a.Aggregate(0, 2, func(i int) (int, error) { return 0, context.Canceled })
		assert.ErrorIs(t, err, context.Canceled)
		r2, _, err := a.Aggregate(0, 1, fn)
		assert.ErrorIs(t, err, context.Canceled)

		// Caller does not have to wait, actually.
		// Testing it to make sure it is not blocking.
		assert.Error(t, r2.Wait(), context.Canceled)
		assert.Error(t, r3.Wait(), context.Canceled)

		assert.Equal(t, uint64(1), a.stats.errors.Load())
	})

	t.Run("Last aggregation failed", func(t *testing.T) {
		a := NewAggregator[int](w, d)
		var start int64
		a.now = func() int64 {
			start += 10
			return start
		}

		_, ok, err := a.Aggregate(0, 0, fn)
		assert.NoError(t, err)
		assert.False(t, ok)

		r2, ok, err := a.Aggregate(0, 1, fn)
		assert.NoError(t, err)
		assert.True(t, ok)

		r3, _, err := a.Aggregate(0, 2, func(i int) (int, error) { return 0, context.Canceled })
		assert.ErrorIs(t, err, context.Canceled)
		assert.ErrorIs(t, r2.Wait(), context.Canceled)
		assert.ErrorIs(t, r3.Wait(), context.Canceled)

		assert.Equal(t, uint64(1), a.stats.errors.Load())
	})
}

func Test_Aggregation_Pruning(t *testing.T) {
	w := time.Second * 15
	d := time.Millisecond * 10

	fn := func(i int) (int, error) {
		return i + 1, nil
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	_, ok, err := a.Aggregate(0, 0, fn)
	assert.NoError(t, err)
	assert.False(t, ok)

	// In order to create aggregate we need at least
	// two requests within the aggregation window.
	r2, ok, err := a.Aggregate(0, 1, fn)
	assert.NoError(t, err)
	assert.True(t, ok)

	assert.NoError(t, r2.Wait())
	// Evict stale aggregates and keys.
	a.prune(int64(d) + a.now())
	assert.Zero(t, len(a.aggregates))
	assert.Zero(t, a.tracker.update(0, 1))
}
