package segmentwriter

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
)

const testInstanceID = "segment-writer-7"

// Asserted literally rather than rebuilt from the constants, so moving the probe is deliberate.
const expectedHealthCheckKey = "segments/_health/" + testInstanceID

func TestSegmentWriterService_performBucketHealthCheck(t *testing.T) {
	t.Parallel()

	const timeout = time.Second

	uploadReturns := func(errs ...error) func(*mockobjstore.MockBucket) {
		return func(bucket *mockobjstore.MockBucket) {
			var attempt int
			bucket.On("Upload", mock.Anything, expectedHealthCheckKey, mock.Anything).
				Return(func(_ context.Context, _ string, r io.Reader, _ ...objstore.ObjectUploadOption) error {
					// A reader shared across retries would be drained after the first.
					body, err := io.ReadAll(r)
					require.NoError(t, err)
					require.Contains(t, string(body), testInstanceID)

					err = errs[min(attempt, len(errs)-1)]
					attempt++
					return err
				})
		}
	}

	uploadThenDelete := func(deleteErr error) func(*mockobjstore.MockBucket) {
		return func(bucket *mockobjstore.MockBucket) {
			uploadReturns(nil)(bucket)
			bucket.On("Delete", mock.Anything, expectedHealthCheckKey).Return(deleteErr)
		}
	}

	unreachable := errors.New("dial tcp: connection refused")
	noSuchBucket := errors.New("NoSuchBucket: the specified bucket does not exist")
	accessDenied := errors.New("AccessDenied: not authorized to perform s3:DeleteObject")

	for _, tc := range []struct {
		name            string
		enabled         bool
		setup           func(*mockobjstore.MockBucket)
		assert          func(*testing.T, error)
		wantCleanupFail int
	}{
		{
			// Skipped entirely: the bucket must not be touched.
			name:    "disabled",
			enabled: false,
			setup:   func(*mockobjstore.MockBucket) {},
			assert:  func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:    "upload and delete succeed",
			enabled: true,
			setup:   uploadThenDelete(nil),
			assert:  func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// A blip must not crash-loop an instance the write path would have tolerated.
			name:    "transient upload failure is retried",
			enabled: true,
			setup: func(bucket *mockobjstore.MockBucket) {
				uploadReturns(unreachable, unreachable, nil)(bucket)
				bucket.On("Delete", mock.Anything, expectedHealthCheckKey).Return(nil)
			},
			assert: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// A bucket granting writes but not deletes is still serviceable.
			name:            "delete permission denied is not fatal",
			enabled:         true,
			setup:           uploadThenDelete(accessDenied),
			assert:          func(t *testing.T, err error) { require.NoError(t, err) },
			wantCleanupFail: 1,
		},
		{
			name:    "delete timeout is not fatal",
			enabled: true,
			setup: func(bucket *mockobjstore.MockBucket) {
				uploadReturns(nil)(bucket)
				bucket.On("Delete", mock.Anything, expectedHealthCheckKey).
					Return(func(ctx context.Context, _ string) error {
						<-ctx.Done()
						return ctx.Err()
					})
			},
			assert:          func(t *testing.T, err error) { require.NoError(t, err) },
			wantCleanupFail: 1,
		},
		{
			name:    "missing bucket is fatal once retries are exhausted",
			enabled: true,
			setup:   uploadReturns(noSuchBucket),
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, noSuchBucket)
				require.ErrorContains(t, err, "bucket health check failed")
			},
		},
		{
			name:    "bucket unreachable is fatal once retries are exhausted",
			enabled: true,
			setup:   uploadReturns(unreachable),
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, unreachable)
				require.ErrorContains(t, err, "bucket health check failed")
			},
		},
		{
			// A real expiring context, so the timeout plumbing itself is covered.
			name:    "upload timeout is fatal",
			enabled: true,
			setup: func(bucket *mockobjstore.MockBucket) {
				bucket.On("Upload", mock.Anything, expectedHealthCheckKey, mock.Anything).
					Return(func(ctx context.Context, _ string, _ io.Reader, _ ...objstore.ObjectUploadOption) error {
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

			svc := newTestServiceForHealthCheck(bucket, tc.enabled, timeout)
			tc.assert(t, svc.performBucketHealthCheck(context.Background()))

			require.Equal(t, float64(tc.wantCleanupFail),
				testutil.ToFloat64(svc.bucketHealthCheckCleanupFailures))
		})
	}
}

// The failure must reach the caller: that is what moves the dskit service to Failed, so the
// instance never registers in the ring.
func TestSegmentWriterService_starting_bucketHealthCheckFailure(t *testing.T) {
	t.Parallel()

	unreachable := errors.New("dial tcp: connection refused")
	bucket := mockobjstore.NewMockBucket(t)
	bucket.On("Upload", mock.Anything, expectedHealthCheckKey, mock.Anything).Return(unreachable)

	// subservices is deliberately left nil: StartManagerAndAwaitHealthy would panic on
	// it, so returning an error instead proves starting bailed out before the ring.
	err := newTestServiceForHealthCheck(bucket, true, time.Second).starting(context.Background())
	require.ErrorIs(t, err, unreachable)
}

func newTestServiceForHealthCheck(bucket *mockobjstore.MockBucket, enabled bool, timeout time.Duration) *SegmentWriterService {
	return &SegmentWriterService{
		logger:        log.NewNopLogger(),
		storageBucket: bucket,
		config: Config{
			LifecyclerConfig:         ring.LifecyclerConfig{ID: testInstanceID},
			BucketHealthCheckEnabled: enabled,
			BucketHealthCheckTimeout: timeout,
			UploadMaxRetries:         3,
			UploadMinBackoff:         time.Millisecond,
			UploadMaxBackoff:         5 * time.Millisecond,
		},
		bucketHealthCheckCleanupFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bucket_health_check_cleanup_failures_total",
		}),
	}
}

// The probe shares segments/ with real block data, so it must never parse as a block: the
// compaction worker deletes keys that resolve to a ULID older than a tombstone's cut-off.
func TestBucketHealthCheckKeyIsNotABlockPath(t *testing.T) {
	t.Parallel()

	_, err := block.ParseBlockIDFromPath(expectedHealthCheckKey)
	require.Error(t, err, "probe key must not parse as a block path")

	// The shard element is a uint32, which is what keeps _health out of any scanned directory.
	_, err = strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(bucketHealthCheckPrefix, block.DirNameSegment+"/"), "/"), 10, 32)
	require.Error(t, err, "probe directory must not be parseable as a shard")
}
