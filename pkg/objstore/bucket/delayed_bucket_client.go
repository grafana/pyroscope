// SPDX-License-Identifier: AGPL-3.0-only

package bucket

import (
	"context"
	"io"
	"math/rand"
	"time"

	"github.com/thanos-io/objstore"
)

// DelayedBucketClient wraps objstore.InstrumentedBucket and add a random delay to each API call.
// This client is intended to be used only for testing purposes.
type DelayedBucketClient struct {
	wrapped  objstore.Bucket
	minDelay time.Duration
	maxDelay time.Duration
}

func NewDelayedBucketClient(wrapped objstore.Bucket, minDelay, maxDelay time.Duration) objstore.Bucket {
	if minDelay < 0 || maxDelay < 0 || maxDelay < minDelay {
		// We're fine just panicking, because we expect this client to be used only for testing purposes.
		panic("invalid delay configuration")
	}

	return &DelayedBucketClient{
		wrapped:  wrapped,
		minDelay: minDelay,
		maxDelay: maxDelay,
	}
}

func (m *DelayedBucketClient) Upload(ctx context.Context, name string, r io.Reader) error {
	m.delay()
	defer m.delay()

	return m.wrapped.Upload(ctx, name, r)
}

func (m *DelayedBucketClient) Delete(ctx context.Context, name string) error {
	m.delay()
	defer m.delay()

	return m.wrapped.Delete(ctx, name)
}

func (m *DelayedBucketClient) Name() string {
	return m.wrapped.Name()
}

func (m *DelayedBucketClient) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	m.delay()
	defer m.delay()

	return m.wrapped.Iter(ctx, dir, f, options...)
}

func (m *DelayedBucketClient) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	m.delay()
	defer m.delay()

	return m.wrapped.Get(ctx, name)
}
func (m *DelayedBucketClient) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	m.delay()
	defer m.delay()

	return m.wrapped.GetRange(ctx, name, off, length)
}

func (m *DelayedBucketClient) Exists(ctx context.Context, name string) (bool, error) {
	m.delay()
	defer m.delay()

	return m.wrapped.Exists(ctx, name)
}

func (m *DelayedBucketClient) IsObjNotFoundErr(err error) bool {
	return m.wrapped.IsObjNotFoundErr(err)
}

func (m *DelayedBucketClient) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	m.delay()
	defer m.delay()

	return m.wrapped.Attributes(ctx, name)
}

func (m *DelayedBucketClient) Close() error {
	return m.wrapped.Close()
}

func (m *DelayedBucketClient) delay() {
	time.Sleep(m.minDelay + time.Duration(rand.Int63n(m.maxDelay.Nanoseconds()-m.minDelay.Nanoseconds())))
}
