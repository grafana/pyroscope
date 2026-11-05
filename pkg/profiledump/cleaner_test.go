package profiledump

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
)

var cleanerNow = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

type cleanupTestBucket struct {
	objstore.Bucket
	iter     func(context.Context, string, func(string) error, ...objstore.IterOption) error
	deleteFn func(context.Context, string) error
}

func (b *cleanupTestBucket) Iter(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
	if b.iter != nil {
		return b.iter(ctx, prefix, f, opts...)
	}
	return b.Bucket.Iter(ctx, prefix, f, opts...)
}
func (b *cleanupTestBucket) Delete(ctx context.Context, key string) error {
	if b.deleteFn != nil {
		return b.deleteFn(ctx, key)
	}
	return b.Bucket.Delete(ctx, key)
}
func newTestCleaner(t *testing.T, cfg CleanerConfig, b CleanupBucket) *Cleaner {
	t.Helper()
	c, err := NewCleaner(cfg, b, log.NewNopLogger(), prometheus.NewRegistry(), func() time.Time { return cleanerNow })
	require.NoError(t, err)
	return c
}
func cleanupKey(t *testing.T, tenant string, when time.Time) string {
	t.Helper()
	key, _, err := NewObjectKey(tenant, when)
	require.NoError(t, err)
	return key
}
func putCleanupKey(t *testing.T, b objstore.Bucket, key string) {
	t.Helper()
	// Deliberately invalid JSON proves cleanup never reads sidecar contents.
	require.NoError(t, b.Upload(context.Background(), key, strings.NewReader(`not JSON or pprof`)))
}
func cleanupExists(t *testing.T, b objstore.Bucket, key string, want bool) {
	t.Helper()
	exists, err := b.Exists(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, want, exists, key)
}

func TestCleanerRetentionBoundaryAndPruning(t *testing.T) {
	for _, tc := range []struct {
		name      string
		retention time.Duration
		cutoff    time.Time
	}{
		{"month", 30 * 24 * time.Hour, time.Date(2026, 8, 19, 12, 30, 0, 0, time.UTC)},
		{"partial hour", 90 * time.Minute, time.Date(2026, 9, 18, 10, 30, 0, 0, time.UTC)},
		{"year boundary", 24 * time.Hour, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"leap day and offset clock", 24 * time.Hour, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retention, cutoff := tc.retention, tc.cutoff
			cfg := CleanerConfig{Retention: retention}
			b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
			var listed []string
			b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
				listed = append(listed, prefix)
				return b.Bucket.Iter(ctx, prefix, f, opts...)
			}
			times := []time.Time{cutoff.Truncate(time.Hour).Add(-time.Millisecond), cutoff.Truncate(time.Hour), cutoff.Truncate(time.Hour).Add(time.Minute), cutoff, cutoff.Add(24 * time.Hour)}
			var keys []string
			for _, when := range times {
				key := cleanupKey(t, "3648", when)
				keys = append(keys, key)
				putCleanupKey(t, b, key)
			}
			c := newTestCleaner(t, cfg, b)
			c.now = func() time.Time { return cutoff.Add(retention).In(time.FixedZone("offset", -7*3600)) }
			c.sweep(context.Background())
			for i, key := range keys {
				cleanupExists(t, b, key, i != 0)
			}
			hour := ObjectPrefix + "3648/" + cutoff.Format("2006-01-02/15") + "/"
			require.NotContains(t, listed, hour)
			c.now = func() time.Time { return cutoff.Truncate(time.Hour).Add(time.Hour).Add(retention) }
			c.sweep(context.Background())
			for i, key := range keys {
				cleanupExists(t, b, key, i == 4)
			}
		})
	}
}

func TestCleanerPairsOrphansAndIsolation(t *testing.T) {
	raw := objstore.NewInMemBucket()
	b := objstore.NewPrefixedBucket(raw, "customer/prefix")
	old := cleanerNow.Add(-8 * 24 * time.Hour)
	pair := cleanupKey(t, "tenant", old)
	parsed, err := ParseObjectKey(pair)
	require.NoError(t, err)
	payloadOnly := cleanupKey(t, "3648", old)
	sidecarOnly := strings.TrimSuffix(cleanupKey(t, "safe-tenant", old), ".pprof") + ".json"
	recent := cleanupKey(t, "tenant", cleanerNow)
	minute := pair[:strings.LastIndex(pair, "/")+1]
	deleted := []string{pair, parsed.MetadataKey, payloadOnly, sidecarOnly,
		minute + "unexpected/deeper.pprof", minute + "invalid.pprof", minute + "../bad.json",
		strings.Replace(recent, cleanerNow.Format("2006-01-02"), old.Format("2006-01-02"), 1),
	}
	preserved := []string{
		"tenant/phlaredb/01DTVP434PA9VFXSW2JKB3392D/profiles.parquet",
		"profile-dumps/phlaredb/01DTVP434PA9VFXSW2JKB3392D/profiles.parquet",
		"__pyroscope_cluster/other-diagnostics/object",
		recent, "unrelated/profiles.parquet", "other/" + pair, ObjectPrefix + "unrelated.txt",
		ObjectPrefix + "tenant/0000/x/object",
		ObjectPrefix + "tenant/0000-01-01/00/object",
		ObjectPrefix + "tenant/2026-09-10/24/object",
		strings.Replace(pair, "/tenant/", "/../", 1),
	}
	for _, key := range append(deleted, preserved...) {
		putCleanupKey(t, b, key)
	}
	putCleanupKey(t, raw, pair)
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(context.Background())
	for _, key := range deleted {
		cleanupExists(t, b, key, false)
	}
	for _, key := range preserved {
		cleanupExists(t, b, key, true)
	}
	cleanupExists(t, raw, pair, true)
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
}

func TestCleanerUnexpectedProviderKeys(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	old := cleanerNow.Add(-8 * 24 * time.Hour)
	key := cleanupKey(t, "a", old)
	outside := cleanupKey(t, "z", old)
	putCleanupKey(t, b, key)
	putCleanupKey(t, b, outside)
	b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
		if prefix == ObjectPrefix {
			// Direct objects may precede prefixes regardless of lexical order.
			require.NoError(t, f(outside))
			return f(ObjectPrefix + "a/")
		}
		if len(opts) != 0 {
			require.NoError(t, f(outside))
		}
		return b.Bucket.Iter(ctx, prefix, f, opts...)
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(context.Background())
	cleanupExists(t, b, key, false)
	cleanupExists(t, b, outside, true)
}

func TestCleanerFailureRecovery(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	old := cleanerNow.Add(-8 * 24 * time.Hour)
	first, second, later := cleanupKey(t, "a", old), cleanupKey(t, "b", old), cleanupKey(t, "z", old)
	for _, key := range []string{first, second, later} {
		putCleanupKey(t, b, key)
	}
	failure := errors.New("first deletion failed")
	secondFailure := errors.New("second deletion failed")
	deletes := 0
	b.deleteFn = func(ctx context.Context, key string) error {
		deletes++
		switch key {
		case first:
			return failure
		case second:
			return secondFailure
		default:
			return b.Bucket.Delete(ctx, key)
		}
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	var logs bytes.Buffer
	c.logger = log.NewLogfmtLogger(&logs)
	c.sweep(context.Background())
	require.Equal(t, 3, deletes)
	cleanupExists(t, b, first, true)
	cleanupExists(t, b, second, true)
	cleanupExists(t, b, later, false)
	require.Zero(t, testutil.ToFloat64(c.success))
	require.Equal(t, 1, strings.Count(logs.String(), "\n"))
	require.Contains(t, logs.String(), failure.Error())
	require.NotContains(t, logs.String(), secondFailure.Error())
	b.deleteFn = nil
	logs.Reset()
	c.sweep(context.Background())
	cleanupExists(t, b, first, false)
	cleanupExists(t, b, second, false)
	require.Empty(t, logs.String())
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
}

func TestCleanerMissingObject(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	key := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, key)
	b.deleteFn = func(ctx context.Context, key string) error {
		require.NoError(t, b.Bucket.Delete(ctx, key))
		return b.Bucket.Delete(ctx, key)
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	require.NoError(t, c.walk(context.Background(), ObjectPrefix, cleanupRootDepth, cleanerNow))
	cleanupExists(t, b, key, false)
}

func TestCleanerDeleteTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
		putCleanupKey(t, b, cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour)))
		b.deleteFn = func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		}
		c := newTestCleaner(t, DefaultCleanerConfig(), b)
		require.ErrorIs(t, c.walk(context.Background(), ObjectPrefix, cleanupRootDepth, cleanerNow), context.DeadlineExceeded)
	})
}

func TestCleanerCancellationDrainsWithoutNewWork(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	old := cleanerNow.Add(-9 * 24 * time.Hour)
	for _, at := range []time.Time{old, old.Add(time.Minute), old.Add(time.Hour), old.Add(24 * time.Hour)} {
		putCleanupKey(t, b, cleanupKey(t, "a", at))
	}
	callbacksAfterCancel, deletes := 0, 0
	b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
		require.NoError(t, ctx.Err(), "no new listing after cancellation")
		// Deliver buffered results even after cancellation, as the S3 producer does.
		err := b.Bucket.Iter(context.Background(), prefix, func(key string) error {
			if ctx.Err() != nil {
				callbacksAfterCancel++
			}
			require.NoError(t, f(key))
			cancel()
			return nil
		}, opts...)
		require.NoError(t, err)
		return ctx.Err()
	}
	b.deleteFn = func(context.Context, string) error {
		require.NoError(t, ctx.Err(), "no new deletion after cancellation")
		deletes++
		cancel()
		return ctx.Err()
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	previousSuccess := float64(cleanerNow.Add(-time.Hour).Unix())
	c.success.Set(previousSuccess)
	var logs bytes.Buffer
	c.logger = log.NewLogfmtLogger(&logs)
	c.sweep(ctx)
	require.Empty(t, logs.String())
	require.Equal(t, 1, deletes)
	require.Positive(t, callbacksAfterCancel)
	require.Equal(t, previousSuccess, testutil.ToFloat64(c.success))
}

func TestCleanerServiceCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
		var calls atomic.Int32
		b.iter = func(context.Context, string, func(string) error, ...objstore.IterOption) error {
			calls.Add(1)
			return nil
		}
		c := newTestCleaner(t, DefaultCleanerConfig(), b)
		require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
		t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c)) })
		synctest.Wait()
		require.Equal(t, int32(1), calls.Load())
		time.Sleep(time.Hour)
		synctest.Wait()
		require.Equal(t, int32(2), calls.Load())
	})
}

func TestCleanerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	c, err := NewCleaner(DefaultCleanerConfig(), b, log.NewNopLogger(), reg, func() time.Time { return cleanerNow })
	require.NoError(t, err)
	putCleanupKey(t, b, ObjectPrefix+"unexpected")
	c.sweep(context.Background())
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
	_, missing := b.Get(context.Background(), "missing")
	require.True(t, b.IsObjNotFoundErr(missing))
	var logs bytes.Buffer
	c.logger = log.NewLogfmtLogger(&logs)
	c.now = func() time.Time { return cleanerNow.Add(time.Hour) }
	for _, failure := range []error{errors.New("root inaccessible"), missing} {
		b.iter = func(context.Context, string, func(string) error, ...objstore.IterOption) error { return failure }
		logs.Reset()
		c.sweep(context.Background())
		require.Contains(t, logs.String(), "list profile dumps")
		require.Contains(t, logs.String(), failure.Error())
		require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
	}
	families, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Equal(t, "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds", families[0].GetName())
	require.Len(t, families[0].Metric, 1)
	require.Empty(t, families[0].Metric[0].Label)

	b.iter = nil
	require.NoError(t, b.Delete(context.Background(), ObjectPrefix+"unexpected"))
	completion := cleanerNow.Add(time.Hour + time.Minute)
	b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
		err := b.Bucket.Iter(ctx, prefix, f, opts...)
		c.now = func() time.Time { return completion }
		return err
	}
	c.sweep(context.Background())
	require.Equal(t, float64(completion.Unix()), testutil.ToFloat64(c.success), "an empty sweep advances the success timestamp to completion time")
}
