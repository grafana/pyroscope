package objstore

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
)

// CountingBucket wraps a Bucket and counts the bytes actually read via Get and
// GetRange calls. The counter is incremented as bytes are consumed from the
// returned ReadCloser, not when the call is issued, so short reads at EOF are
// not over-counted. Download is covered implicitly because objstore.Download
// calls Get internally.
//
// An internal WaitGroup tracks every open reader returned by Get and GetRange.
// Call Wait to block until all readers have been closed and their bytes fully
// counted. This is important when the bucket is used with async readers (e.g.
// parquet in ReadModeAsync) whose goroutines may still be draining a reader
// after the outer errgroup has returned.
//
// Attributes is not intercepted (it carries no payload bytes).
type CountingBucket struct {
	Bucket
	bytesRead *atomic.Uint64
	wg        sync.WaitGroup
}

func NewCountingBucket(bkt Bucket, counter *atomic.Uint64) *CountingBucket {
	return &CountingBucket{Bucket: bkt, bytesRead: counter}
}

// Wait blocks until every reader previously returned by Get or GetRange has
// been closed. Call this after all query goroutines have finished to ensure
// the byte counter is stable before reading it.
func (c *CountingBucket) Wait() {
	c.wg.Wait()
}

func (c *CountingBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	rc, err := c.Bucket.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	c.wg.Add(1)
	return &countingReadCloser{ReadCloser: rc, counter: c.bytesRead, wg: &c.wg}, nil
}

func (c *CountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	rc, err := c.Bucket.GetRange(ctx, name, off, length)
	if err != nil {
		return nil, err
	}
	c.wg.Add(1)
	return &countingReadCloser{ReadCloser: rc, counter: c.bytesRead, wg: &c.wg}, nil
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
// of bytes returned from each Read call. It decrements the WaitGroup on Close
// so that CountingBucket.Wait can synchronise with async readers.
type countingReadCloser struct {
	io.ReadCloser
	counter *atomic.Uint64
	wg      *sync.WaitGroup
	once    sync.Once
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.counter.Add(uint64(n))
	}
	return n, err
}

func (r *countingReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(r.wg.Done)
	return err
}
