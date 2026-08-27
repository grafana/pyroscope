package segmentwriter

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
)

func TestSegmentWriterService_performBucketHealthCheck(t *testing.T) {
	t.Parallel()

	const timeout = 100 * time.Millisecond

	// iterCalledWith returns a mock expectation for Iter at the bucket root, invoking
	// visit with the callback the health check passed in.
	iterCalledWith := func(visit func(f func(string) error) error) func(*mockobjstore.MockBucket) {
		return func(bucket *mockobjstore.MockBucket) {
			bucket.On("Iter", mock.Anything, "", mock.Anything).
				Return(func(_ context.Context, _ string, f func(string) error, _ ...objstore.IterOption) error {
					return visit(f)
				})
		}
	}

	// iterFailsWith makes Iter return err without invoking the callback.
	iterFailsWith := func(err error) func(*mockobjstore.MockBucket) {
		return iterCalledWith(func(func(string) error) error { return err })
	}

	unreachable := errors.New("dial tcp: connection refused")
	noSuchBucket := errors.New("NoSuchBucket: the specified bucket does not exist")

	for _, tc := range []struct {
		name    string
		enabled bool
		setup   func(*mockobjstore.MockBucket)
		assert  func(*testing.T, error)
	}{
		{
			// The check is skipped entirely: the bucket must not be touched.
			name:    "disabled",
			enabled: false,
			setup:   func(*mockobjstore.MockBucket) {},
			assert:  func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// One of only two healthy outcomes: an empty bucket never invokes the
			// callback, so Iter returns nil.
			name:    "empty bucket",
			enabled: true,
			setup:   iterCalledWith(func(func(string) error) error { return nil }),
			assert:  func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// The other healthy outcome, and the production shape: the callback fires and
			// its sentinel comes back out of Iter. Treating that as a failure would stop
			// the segment writer starting against a perfectly healthy bucket.
			name:    "non-empty bucket",
			enabled: true,
			setup: iterCalledWith(func(f func(string) error) error {
				return f("some/object/path")
			}),
			assert: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// The sentinel must be recognised even if a bucket implementation wraps it.
			// This is what errors.Is buys over comparing the message.
			name:    "non-empty bucket with wrapped sentinel",
			enabled: true,
			setup: iterCalledWith(func(f func(string) error) error {
				return fmt.Errorf("iter some/prefix: %w", f("some/object/path"))
			}),
			assert: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// A nonexistent bucket must not start. This is why the success condition
			// cannot accept "object not found": a list never names a key, so that class
			// only ever arrives from a missing bucket, and Tencent COS reports any 404 -
			// NoSuchBucket included - as a not-found.
			name:    "missing bucket is fatal",
			enabled: true,
			setup:   iterFailsWith(noSuchBucket),
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, noSuchBucket)
				require.ErrorContains(t, err, "bucket health check failed")
			},
		},
		{
			// Anything else stops the segment writer starting too, rather than letting it
			// report itself healthy and drop writes it cannot fulfil. The cause is wrapped
			// so operators can see it.
			name:    "bucket unreachable is fatal",
			enabled: true,
			setup:   iterFailsWith(unreachable),
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, unreachable)
				require.ErrorContains(t, err, "bucket health check failed")
			},
		},
		{
			// Exercised through a real expiring context rather than a synthetic error,
			// so the timeout plumbing itself is covered.
			name:    "timeout is fatal",
			enabled: true,
			setup: func(bucket *mockobjstore.MockBucket) {
				bucket.On("Iter", mock.Anything, "", mock.Anything).
					Return(func(ctx context.Context, _ string, _ func(string) error, _ ...objstore.IterOption) error {
						<-ctx.Done()
						return ctx.Err()
					})
			},
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.ErrorContains(t, err, "bucket health check failed")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bucket := mockobjstore.NewMockBucket(t)
			tc.setup(bucket)

			svc := &SegmentWriterService{
				logger:        log.NewNopLogger(),
				storageBucket: bucket,
				config: Config{
					BucketHealthCheckEnabled: tc.enabled,
					BucketHealthCheckTimeout: timeout,
				},
			}

			tc.assert(t, svc.performBucketHealthCheck(context.Background()))
		})
	}
}

// The failure has to reach the caller so the dskit service transitions to Failed, which is
// what stops the instance registering in the ring and ultimately exits the process.
func TestSegmentWriterService_starting_bucketHealthCheckFailure(t *testing.T) {
	t.Parallel()

	unreachable := errors.New("dial tcp: connection refused")
	bucket := mockobjstore.NewMockBucket(t)
	bucket.On("Iter", mock.Anything, "", mock.Anything).Return(unreachable)

	svc := &SegmentWriterService{
		logger:        log.NewNopLogger(),
		storageBucket: bucket,
		config: Config{
			BucketHealthCheckEnabled: true,
			BucketHealthCheckTimeout: time.Second,
		},
	}

	// subservices is deliberately left nil: StartManagerAndAwaitHealthy would panic on
	// it, so returning an error instead proves starting bailed out before the ring.
	err := svc.starting(context.Background())
	require.ErrorIs(t, err, unreachable)
}
