package inflight

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimiter_Reserve(t *testing.T) {
	t.Parallel()

	t.Run("disabled limiter accepts everything but still accounts", func(t *testing.T) {
		l := NewLimiter(0, nil)
		r, ok := l.Reserve(1 << 20)
		require.True(t, ok)
		assert.Equal(t, int64(1<<20), l.Bytes())
		r.Release()
		assert.Equal(t, int64(0), l.Bytes())
	})

	t.Run("reservation at the limit is accepted", func(t *testing.T) {
		l := NewLimiter(100, nil)
		_, ok := l.Reserve(100)
		require.True(t, ok)
	})

	t.Run("reservation over the limit is rejected but accounted", func(t *testing.T) {
		l := NewLimiter(100, nil)
		first, ok := l.Reserve(60)
		require.True(t, ok)

		second, ok := l.Reserve(60)
		require.False(t, ok)
		assert.Equal(t, int64(120), l.Bytes())

		// Releasing the rejected reservation makes room again.
		second.Release()
		assert.Equal(t, int64(60), l.Bytes())
		third, ok := l.Reserve(40)
		require.True(t, ok)

		third.Release()
		first.Release()
		assert.Equal(t, int64(0), l.Bytes())
	})
}

type recordingObserver []float64

func (o *recordingObserver) Observe(v float64) { *o = append(*o, v) }

func TestLimiter_HighWatermark(t *testing.T) {
	t.Parallel()

	var observed recordingObserver
	l := NewLimiter(100, &observed)

	first, _ := l.Reserve(60)
	second, ok := l.Reserve(60)
	require.False(t, ok)

	second.Release()
	first.Release()
	third, _ := l.Reserve(10)
	third.Release()

	// The peak is sampled when a reservation is taken, so it survives even
	// though the limiter is back to zero by the time the test reads it.
	assert.Equal(t, recordingObserver{60, 120, 10}, observed)
	assert.Equal(t, int64(0), l.Bytes())
}

func TestReservation_References(t *testing.T) {
	t.Parallel()

	l := NewLimiter(100, nil)
	r, ok := l.Reserve(100)
	require.True(t, ok)

	require.True(t, r.Retain())
	r.Release()
	assert.Equal(t, int64(100), l.Bytes(), "bytes are held until the last reference is released")

	r.Release()
	assert.Equal(t, int64(0), l.Bytes())

	require.False(t, r.Retain(), "a fully released reservation cannot be retained")
	assert.Equal(t, int64(0), l.Bytes())
}

func TestReservation_Concurrent(t *testing.T) {
	t.Parallel()

	l := NewLimiter(0, nil)
	r, _ := l.Reserve(1000)

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		require.True(t, r.Retain())
		go func() {
			defer wg.Done()
			r.Release()
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1000), l.Bytes())

	r.Release()
	assert.Equal(t, int64(0), l.Bytes())
}

func TestDetach(t *testing.T) {
	t.Parallel()

	t.Run("without a reservation in context", func(t *testing.T) {
		release := Detach(context.Background())
		require.NotNil(t, release)
		release()
	})

	t.Run("with a nil reservation in context", func(t *testing.T) {
		ctx := NewContext(context.Background(), nil)
		assert.Nil(t, FromContext(ctx))
		Detach(ctx)()
	})

	t.Run("outlives the request", func(t *testing.T) {
		l := NewLimiter(100, nil)
		r, _ := l.Reserve(100)
		ctx := NewContext(context.Background(), r)

		release := Detach(ctx)
		r.Release() // The request returns.
		assert.Equal(t, int64(100), l.Bytes())

		release()
		assert.Equal(t, int64(0), l.Bytes())
	})

	t.Run("detaching a released reservation is a no-op", func(t *testing.T) {
		l := NewLimiter(100, nil)
		r, _ := l.Reserve(100)
		ctx := NewContext(context.Background(), r)
		r.Release()

		Detach(ctx)()
		assert.Equal(t, int64(0), l.Bytes())
	})
}
