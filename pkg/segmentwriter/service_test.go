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

// Asserted literally rather than rebuilt from the constants, so moving the probe is deliberate:
// it has to stay on the prefix real writes use, or a prefix-scoped policy could pass the check
// and deny every write.
const testHealthCheckPrefix = "segments/_health/" + testInstanceID + "/"

// probe records the keys the health check touched, which lets every case assert both how many
// attempts it made and that the cleanup targeted the object it had just uploaded.
type probe struct {
	uploaded []string
	deleted  []string
}

func (p *probe) onUpload(t *testing.T, bucket *mockobjstore.MockBucket, err error) {
	bucket.On("Upload", mock.Anything, healthCheckKey(), mock.Anything).
		Return(func(_ context.Context, name string, r io.Reader, _ ...objstore.ObjectUploadOption) error {
			body, readErr := io.ReadAll(r)
			require.NoError(t, readErr)
			require.Contains(t, string(body), testInstanceID)
			p.uploaded = append(p.uploaded, name)
			return err
		})
}

func (p *probe) onDelete(bucket *mockobjstore.MockBucket, fn func(context.Context) error) {
	bucket.On("Delete", mock.Anything, healthCheckKey()).
		Return(func(ctx context.Context, name string) error {
			p.deleted = append(p.deleted, name)
			return fn(ctx)
		})
}

func healthCheckKey() interface{} {
	return mock.MatchedBy(func(name string) bool {
		return strings.HasPrefix(name, testHealthCheckPrefix) && len(name) > len(testHealthCheckPrefix)
	})
}

func TestSegmentWriterService_performBucketHealthCheck(t *testing.T) {
	t.Parallel()

	const timeout = 100 * time.Millisecond

	unreachable := errors.New("dial tcp: connection refused")
	noSuchBucket := errors.New("NoSuchBucket: the specified bucket does not exist")
	accessDenied := errors.New("AccessDenied: not authorized to perform s3:DeleteObject")
	succeeds := func(context.Context) error { return nil }
	fails := func(err error) func(context.Context) error {
		return func(context.Context) error { return err }
	}
	blocksUntilCancelled := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}

	for _, tc := range []struct {
		name            string
		enabled         bool
		setup           func(*testing.T, *mockobjstore.MockBucket, *probe)
		assert          func(*testing.T, error)
		wantUploads     int
		wantDeletes     int
		wantCleanupFail int
	}{
		{
			// Skipped entirely: the bucket must not be touched.
			name:    "disabled",
			enabled: false,
			setup:   func(*testing.T, *mockobjstore.MockBucket, *probe) {},
			assert:  func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:    "upload and delete succeed",
			enabled: true,
			setup: func(t *testing.T, bucket *mockobjstore.MockBucket, p *probe) {
				p.onUpload(t, bucket, nil)
				bucket.On("Provider").Return(objstore.S3)
				p.onDelete(bucket, succeeds)
			},
			assert:      func(t *testing.T, err error) { require.NoError(t, err) },
			wantUploads: 1,
			wantDeletes: 1,
		},
		{
			// A bucket granting writes but not deletes is still serviceable.
			name:    "delete permission denied is not fatal",
			enabled: true,
			setup: func(t *testing.T, bucket *mockobjstore.MockBucket, p *probe) {
				p.onUpload(t, bucket, nil)
				bucket.On("Provider").Return(objstore.S3)
				p.onDelete(bucket, fails(accessDenied))
			},
			assert:          func(t *testing.T, err error) { require.NoError(t, err) },
			wantUploads:     1,
			wantDeletes:     1,
			wantCleanupFail: 1,
		},
		{
			name:    "delete timeout is not fatal",
			enabled: true,
			setup: func(t *testing.T, bucket *mockobjstore.MockBucket, p *probe) {
				p.onUpload(t, bucket, nil)
				bucket.On("Provider").Return(objstore.S3)
				p.onDelete(bucket, blocksUntilCancelled)
			},
			assert:          func(t *testing.T, err error) { require.NoError(t, err) },
			wantUploads:     1,
			wantDeletes:     1,
			wantCleanupFail: 1,
		},
		{
			// One attempt only: the restart is the retry, and the supervisor backs it off.
			name:    "bucket unreachable is fatal and not retried",
			enabled: true,
			setup: func(t *testing.T, bucket *mockobjstore.MockBucket, p *probe) {
				p.onUpload(t, bucket, unreachable)
			},
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, unreachable)
				require.ErrorContains(t, err, "bucket health check failed")
			},
			wantUploads: 1,
		},
		{
			name:    "missing bucket is fatal",
			enabled: true,
			setup: func(t *testing.T, bucket *mockobjstore.MockBucket, p *probe) {
				p.onUpload(t, bucket, noSuchBucket)
			},
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, noSuchBucket)
				require.ErrorContains(t, err, "bucket health check failed")
			},
			wantUploads: 1,
		},
		{
			// A real expiring context, so the timeout plumbing itself is covered.
			name:    "upload timeout is fatal",
			enabled: true,
			setup: func(t *testing.T, bucket *mockobjstore.MockBucket, p *probe) {
				bucket.On("Upload", mock.Anything, healthCheckKey(), mock.Anything).
					Return(func(ctx context.Context, name string, _ io.Reader, _ ...objstore.ObjectUploadOption) error {
						p.uploaded = append(p.uploaded, name)
						<-ctx.Done()
						return ctx.Err()
					})
			},
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.ErrorContains(t, err, "bucket health check failed")
			},
			wantUploads: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bucket := mockobjstore.NewMockBucket(t)
			var p probe
			tc.setup(t, bucket, &p)

			svc := newTestServiceForHealthCheck(bucket, tc.enabled, timeout)
			tc.assert(t, svc.performBucketHealthCheck(context.Background()))

			require.Len(t, p.uploaded, tc.wantUploads)
			require.Len(t, p.deleted, tc.wantDeletes)
			if len(p.deleted) > 0 {
				require.Equal(t, p.uploaded, p.deleted, "cleanup must target the object just uploaded")
			}
			require.Equal(t, float64(tc.wantCleanupFail),
				testutil.ToFloat64(svc.bucketHealthCheckCleanupFailures))
		})
	}
}

// The filesystem bucket deletes an object by walking up and removing each parent it has just
// found empty, so the cleanup could take out a segment another writer created in that window.
// Leaving a stray file behind is the cheaper trade. No Delete expectation is registered, so
// the mock fails the test if the check attempts one.
func TestSegmentWriterService_performBucketHealthCheck_filesystemKeepsProbe(t *testing.T) {
	t.Parallel()

	bucket := mockobjstore.NewMockBucket(t)
	var p probe
	p.onUpload(t, bucket, nil)
	bucket.On("Provider").Return(objstore.FILESYSTEM)

	svc := newTestServiceForHealthCheck(bucket, true, time.Second)
	require.NoError(t, svc.performBucketHealthCheck(context.Background()))

	require.Len(t, p.uploaded, 1)
	require.Empty(t, p.deleted)
	require.Zero(t, testutil.ToFloat64(svc.bucketHealthCheckCleanupFailures))
}

// A grant such as GCS roles/storage.objectCreator can create an object and not replace one, so
// a probe key that repeated across restarts would fail every start after the first. Real writes
// only ever create, so the check must not need more than that.
func TestSegmentWriterService_performBucketHealthCheck_createOnlyBucket(t *testing.T) {
	t.Parallel()

	overwriteDenied := errors.New("AccessDenied: replacing an object requires storage.objects.delete")
	seen := make(map[string]struct{})

	var uploaded []string
	bucket := mockobjstore.NewMockBucket(t)
	bucket.On("Upload", mock.Anything, healthCheckKey(), mock.Anything).
		Return(func(_ context.Context, name string, _ io.Reader, _ ...objstore.ObjectUploadOption) error {
			if _, ok := seen[name]; ok {
				return overwriteDenied
			}
			seen[name] = struct{}{}
			uploaded = append(uploaded, name)
			return nil
		})
	// A bucket that refuses overwrites refuses deletes too: both need the same permission.
	bucket.On("Provider").Return(objstore.GCS)
	bucket.On("Delete", mock.Anything, healthCheckKey()).Return(overwriteDenied)

	svc := newTestServiceForHealthCheck(bucket, true, time.Second)
	require.NoError(t, svc.performBucketHealthCheck(context.Background()))
	require.NoError(t, svc.performBucketHealthCheck(context.Background()))

	require.Len(t, uploaded, 2)
	require.NotEqual(t, uploaded[0], uploaded[1], "each start must use a fresh key")
	require.Equal(t, float64(2), testutil.ToFloat64(svc.bucketHealthCheckCleanupFailures))
}

// The failure must reach the caller: that is what moves the dskit service to Failed, so the
// instance never registers in the ring.
func TestSegmentWriterService_starting_bucketHealthCheckFailure(t *testing.T) {
	t.Parallel()

	unreachable := errors.New("dial tcp: connection refused")
	bucket := mockobjstore.NewMockBucket(t)
	bucket.On("Upload", mock.Anything, healthCheckKey(), mock.Anything).Return(unreachable)

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

	key := bucketHealthCheckKey(testInstanceID)
	require.True(t, strings.HasPrefix(key, testHealthCheckPrefix))

	_, err := block.ParseBlockIDFromPath(key)
	require.Error(t, err, "probe key must not parse as a block path")

	// The shard element is a uint32, which is what keeps _health out of any scanned directory.
	_, err = strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(bucketHealthCheckPrefix, block.DirNameSegment+"/"), "/"), 10, 32)
	require.Error(t, err, "probe directory must not be parseable as a shard")
}
