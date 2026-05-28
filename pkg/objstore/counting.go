package objstore

import (
	"context"
	"io"
	"sync/atomic"
)

// CountingBucket wraps a Bucket and counts the bytes actually read via Get and
// GetRange calls. The counter is incremented as bytes are consumed from the
// returned ReadCloser, not when the call is issued, so short reads at EOF are
// not over-counted. Download is covered implicitly because objstore.Download
// calls Get internally.
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

func (c *CountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	rc, err := c.Bucket.GetRange(ctx, name, off, length)
	if err != nil {
		return nil, err
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
// of bytes returned from each Read call.
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
