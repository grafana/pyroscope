package objstore

import (
	"context"
	"io"
	"sync/atomic"
)

// CountingBucket wraps a Bucket and counts the bytes requested via GetRange
// calls. The counter is incremented synchronously when GetRange is called,
// using the requested length. This makes the count deterministic and independent
// of async reader goroutines that may read the returned stream after the caller's
// main goroutine has already moved on.
//
// For Get (unknown payload size), bytes are still counted as consumed from the
// returned ReadCloser, which is sequentially read to EOF in all call sites.
//
// Attributes is not intercepted (it carries no payload bytes).
type CountingBucket struct {
	Bucket
	bytesRead *atomic.Uint64
}

func NewCountingBucket(bkt Bucket, counter *atomic.Uint64) *CountingBucket {
	return &CountingBucket{Bucket: bkt, bytesRead: counter}
}

func (c *CountingBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	rc, err := c.Bucket.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return &countingReadCloser{ReadCloser: rc, counter: c.bytesRead}, nil
}

// GetRange counts the requested length synchronously on success, before the
// caller reads any bytes. This ensures the count is stable regardless of when
// (or whether) the returned reader is drained by an async goroutine.
// When length <= 0 (read-to-EOF semantics), bytes are counted lazily via the
// returned reader, as the total is unknown at call time.
func (c *CountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	rc, err := c.Bucket.GetRange(ctx, name, off, length)
	if err != nil {
		return nil, err
	}
	if length > 0 {
		c.bytesRead.Add(uint64(length))
		return rc, nil
	}
	return &countingReadCloser{ReadCloser: rc, counter: c.bytesRead}, nil
}

// ReaderAt returns a ReaderAtCloser backed by this bucket's GetRange so that
// reads via ReaderAt are also counted.
func (c *CountingBucket) ReaderAt(ctx context.Context, name string) (ReaderAtCloser, error) {
	return &ReaderAt{
		GetRangeReader: c,
		name:           name,
		ctx:            ctx,
	}, nil
}

// countingReadCloser wraps a ReadCloser and increments counter by the number
// of bytes returned from each Read call. Used for Get calls where the payload
// size is not known in advance.
type countingReadCloser struct {
	io.ReadCloser
	counter *atomic.Uint64
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.counter.Add(uint64(n))
	}
	return n, err
}
