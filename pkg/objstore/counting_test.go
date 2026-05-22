package objstore_test

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

func Test_CountingBucket_Get(t *testing.T) {
	inner := memory.NewInMemBucket()
	ctx := context.Background()
	data := []byte("hello world")
	require.NoError(t, inner.Upload(ctx, "test", bytes.NewReader(data)))

	var counter atomic.Uint64
	bkt := objstore.NewCountingBucket(objstore.NewBucket(inner), &counter)

	rc, err := bkt.Get(ctx, "test")
	require.NoError(t, err)
	_, err = io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Equal(t, uint64(len(data)), counter.Load())

	// A failed Get must not increment the counter.
	_, err = bkt.Get(ctx, "missing")
	require.Error(t, err)
	require.Equal(t, uint64(len(data)), counter.Load(), "failed reads must not count")
}

func Test_CountingBucket_GetRange(t *testing.T) {
	inner := memory.NewInMemBucket()
	ctx := context.Background()
	data := []byte("hello world")
	require.NoError(t, inner.Upload(ctx, "test", bytes.NewReader(data)))

	var counter atomic.Uint64
	bkt := objstore.NewCountingBucket(objstore.NewBucket(inner), &counter)

	rc, err := bkt.GetRange(ctx, "test", 0, int64(len(data)))
	require.NoError(t, err)
	_, err = io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Equal(t, uint64(len(data)), counter.Load())

	// A failed GetRange must not increment the counter.
	_, err = bkt.GetRange(ctx, "missing", 0, 5)
	require.Error(t, err)
	require.Equal(t, uint64(len(data)), counter.Load(), "failed reads must not count")
}

func Test_CountingBucket_ReaderAt(t *testing.T) {
	inner := memory.NewInMemBucket()
	ctx := context.Background()
	data := []byte("hello world")
	require.NoError(t, inner.Upload(ctx, "test", bytes.NewReader(data)))

	var counter atomic.Uint64
	bkt := objstore.NewCountingBucket(objstore.NewBucket(inner), &counter)

	ra, err := bkt.ReaderAt(ctx, "test")
	require.NoError(t, err)
	defer func() { require.NoError(t, ra.Close()) }()

	buf := make([]byte, 5)
	n, err := ra.ReadAt(buf, 0)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, uint64(5), counter.Load())

	n, err = ra.ReadAt(buf, 6)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, uint64(10), counter.Load(), "second read must add to counter")
}

func Test_CountingBucket_IndependentPerInvoke(t *testing.T) {
	// Simulates the "retried calls do not accumulate" guarantee: each Invoke
	// creates its own counter, so two independent invocations report their own
	// bytes, not a running total.
	inner := memory.NewInMemBucket()
	ctx := context.Background()
	data := []byte("hello world")
	require.NoError(t, inner.Upload(ctx, "test", bytes.NewReader(data)))

	base := objstore.NewBucket(inner)

	invoke := func() uint64 {
		var counter atomic.Uint64
		bkt := objstore.NewCountingBucket(base, &counter)
		rc, err := bkt.GetRange(ctx, "test", 0, int64(len(data)))
		require.NoError(t, err)
		_, err = io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		return counter.Load()
	}

	first := invoke()
	second := invoke()
	require.Equal(t, uint64(len(data)), first)
	require.Equal(t, first, second, "independent invokes must report the same bytes")
}

// Ensure CountingBucket satisfies the Bucket interface at compile time.
var _ objstore.Bucket = (*objstore.CountingBucket)(nil)
