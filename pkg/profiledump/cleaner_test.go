package profiledump

import (
	"context"
	"errors"
	"flag"

	"io"
	"strings"

	"testing"
	"time"

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
func (*cleanupTestBucket) Get(context.Context, string) (io.ReadCloser, error) { panic("payload read") }
func (*cleanupTestBucket) Attributes(context.Context, string) (objstore.ObjectAttributes, error) {
	panic("attribute read")
}
func (*cleanupTestBucket) Close() error { panic("borrowed storage closed") }

func newTestCleaner(t *testing.T, cfg CleanerConfig, b CleanupBucket) *Cleaner {
	t.Helper()
	c, err := NewCleaner(cfg, b, prometheus.NewRegistry(), func() time.Time { return cleanerNow })
	require.NoError(t, err)
	return c
}
func cleanupKey(t *testing.T, tenant string, when time.Time) string {
	t.Helper()
	key, _, err := NewNativeObjectKey(tenant, when)
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
	for _, retention := range []time.Duration{7 * 24 * time.Hour, 30 * 24 * time.Hour, 90 * time.Minute} {
		t.Run(retention.String(), func(t *testing.T) {
			cfg := CleanerConfig{Retention: retention}
			cutoff := cleanerNow.Add(-retention).Truncate(time.Hour).Add(30 * time.Minute)
			b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
			var listed []string
			b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
				listed = append(listed, prefix)
				return b.Bucket.Iter(ctx, prefix, f, opts...)
			}
			times := []time.Time{cutoff.Truncate(time.Hour).Add(-time.Millisecond), cutoff.Truncate(time.Hour), cutoff.Add(-time.Millisecond), cutoff, cutoff.Add(24 * time.Hour)}
			var keys []string
			for _, when := range times {
				key := cleanupKey(t, "3648", when)
				keys = append(keys, key)
				putCleanupKey(t, b, key)
			}
			c := newTestCleaner(t, cfg, b)
			c.now = func() time.Time { return cutoff.Add(retention) }
			c.sweep(context.Background())
			for i, key := range keys {
				cleanupExists(t, b, key, i != 0)
			}
			hour := NativeObjectPrefix + "3648/" + cutoff.Format("2006-01-02/15") + "/"
			require.NotContains(t, listed, hour)
			require.Equal(t, 1.0, testutil.ToFloat64(c.deleted))
			c.now = func() time.Time { return cutoff.Truncate(time.Hour).Add(time.Hour).Add(retention) }
			c.sweep(context.Background())
			for i, key := range keys {
				cleanupExists(t, b, key, i == 4)
			}
		})
	}
}

func TestCleanerNativePairsOrphansAndIsolation(t *testing.T) {
	raw := objstore.NewInMemBucket()
	b := &cleanupTestBucket{Bucket: objstore.NewPrefixedBucket(raw, "customer/prefix")}
	old := cleanerNow.Add(-8 * 24 * time.Hour)
	pair := cleanupKey(t, "tenant", old)
	parsed, err := ParseNativeObjectKey(pair)
	require.NoError(t, err)
	payloadOnly := cleanupKey(t, "3648", old)
	sidecarOnly := strings.TrimSuffix(cleanupKey(t, "safe-tenant", old), ".pprof") + ".json"
	deleted := []string{pair, parsed.MetadataKey, payloadOnly, sidecarOnly}
	recent := cleanupKey(t, "tenant", cleanerNow)
	minute := pair[:strings.LastIndex(pair, "/")+1]
	preserved := []string{
		"tenant/phlaredb/01DTVP434PA9VFXSW2JKB3392D/profiles.parquet",
		"profile-debug-dumps/phlaredb/01DTVP434PA9VFXSW2JKB3392D/profiles.parquet",
		"__pyroscope_cluster/other-diagnostics/object",
		recent, "unrelated/profiles.parquet", "other/" + pair, ObjectPrefix + "unrelated.txt",
		pair + ".json", strings.Replace(pair, ".pprof", ".PPROF", 1),
		minute + "unexpected/deeper.pprof", minute + "invalid.pprof", minute + "../bad.json",
		strings.Replace(pair, "2026-09-10", "2026-02-30", 1),
		strings.Replace(pair, "/12/", "/1/", 1),
		strings.Replace(pair, "/tenant/", "/../", 1),
		strings.Replace(pair, "/tenant/", "//", 1),
		strings.Replace(recent, cleanerNow.Format("2006-01-02"), old.Format("2006-01-02"), 1),
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
	require.Equal(t, float64(len(deleted)), testutil.ToFloat64(c.deleted))
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
		if prefix == NativeObjectPrefix {
			// Direct objects may precede prefixes regardless of lexical order.
			require.NoError(t, f(outside))
			return f(NativeObjectPrefix + "a/")
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

func TestCleanerDeleteFailuresAndMissing(t *testing.T) {
	for _, failure := range []string{"error", "missing", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
			failed := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
			later := cleanupKey(t, "z", cleanerNow.Add(-8*24*time.Hour))
			putCleanupKey(t, b, failed)
			putCleanupKey(t, b, later)
			b.deleteFn = func(ctx context.Context, key string) error {
				if key == failed {
					switch failure {
					case "error":
						return errors.New("synthetic failure")
					case "missing":
						require.NoError(t, b.Bucket.Delete(ctx, key))
					case "timeout":
						<-ctx.Done()
						return ctx.Err()
					}
				}
				return b.Bucket.Delete(ctx, key)
			}
			c := newTestCleaner(t, DefaultCleanerConfig(), b)
			require.Equal(t, 10*time.Second, c.deleteTimeout)
			c.deleteTimeout = 10 * time.Millisecond
			c.sweep(context.Background())
			cleanupExists(t, b, later, false)
			require.Equal(t, 1.0, testutil.ToFloat64(c.deleted))
			if failure == "missing" {
				require.Zero(t, testutil.ToFloat64(c.errors.WithLabelValues("delete")))
				require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
			} else {
				cleanupExists(t, b, failed, true)
				require.Equal(t, 1.0, testutil.ToFloat64(c.errors.WithLabelValues("delete")))
				require.Zero(t, testutil.ToFloat64(c.success))
				b.deleteFn = nil
				c.sweep(context.Background())
				cleanupExists(t, b, failed, false)
				require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
			}
		})
	}
}

func TestCleanerListingFailures(t *testing.T) {
	for _, level := range []string{"root", "tenant", "date", "hour"} {
		t.Run(level, func(t *testing.T) {
			b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
			old := cleanerNow.Add(-9 * 24 * time.Hour)
			failed := cleanupKey(t, "a", old)
			laterHour := cleanupKey(t, "a", old.Add(time.Hour))
			laterDate := cleanupKey(t, "a", old.Add(24*time.Hour))
			laterTenant := cleanupKey(t, "z", old)
			keys := []string{failed, laterHour, laterDate, laterTenant}
			for _, key := range keys {
				putCleanupKey(t, b, key)
			}
			prefix := NativeObjectPrefix
			if level != "root" {
				prefix += "a/"
			}
			if level == "date" || level == "hour" {
				prefix += old.Format("2006-01-02") + "/"
			}
			if level == "hour" {
				prefix += old.Format("15") + "/"
			}
			b.iter = func(ctx context.Context, dir string, f func(string) error, opts ...objstore.IterOption) error {
				if dir == prefix {
					return errors.New("listing failed")
				}
				return b.Bucket.Iter(ctx, dir, f, opts...)
			}
			c := newTestCleaner(t, DefaultCleanerConfig(), b)
			c.sweep(context.Background())
			for _, key := range keys {
				cleanupExists(t, b, key, strings.HasPrefix(key, prefix))
			}
			require.Equal(t, 1.0, testutil.ToFloat64(c.errors.WithLabelValues("list")))
			require.Zero(t, testutil.ToFloat64(c.success))
			b.iter = nil
			c.sweep(context.Background())
			for _, key := range keys {
				cleanupExists(t, b, key, false)
			}
			require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
		})
	}
}

func TestCleanerFixedCutoffAndLateArrivals(t *testing.T) {
	cfg := DefaultCleanerConfig()
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	aging := cleanupKey(t, "a", cleanerNow.Add(-cfg.Retention))
	late := cleanupKey(t, "a", cleanerNow.Add(-cfg.Retention-time.Hour))
	putCleanupKey(t, b, aging)
	c := newTestCleaner(t, cfg, b)
	b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
		c.now = func() time.Time { return cleanerNow.Add(time.Hour) }
		return b.Bucket.Iter(ctx, prefix, f, opts...)
	}
	c.sweep(context.Background())
	cleanupExists(t, b, aging, true)
	putCleanupKey(t, b, late)
	c.sweep(context.Background())
	cleanupExists(t, b, aging, false)
	cleanupExists(t, b, late, false)
}

func TestCleanerUTCDateBoundaries(t *testing.T) {
	for _, cutoff := range []time.Time{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)} {
		t.Run(cutoff.String(), func(t *testing.T) {
			cfg := DefaultCleanerConfig()
			b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
			old := cleanupKey(t, "tenant", cutoff.Add(-time.Millisecond))
			equal := cleanupKey(t, "tenant", cutoff)
			putCleanupKey(t, b, old)
			putCleanupKey(t, b, equal)
			c := newTestCleaner(t, cfg, b)
			c.now = func() time.Time { return cutoff.Add(cfg.Retention).In(time.FixedZone("offset", -7*3600)) }
			c.sweep(context.Background())
			cleanupExists(t, b, old, false)
			cleanupExists(t, b, equal, true)
		})
	}
}

func TestCleanerCancellationDrainsWithoutNewDeletes(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := cleanerNow.Add(-8 * 24 * time.Hour)
	keys := []string{cleanupKey(t, "a", old), cleanupKey(t, "a", old.Add(time.Minute))}
	for _, key := range keys {
		putCleanupKey(t, b, key)
	}
	callbacks, deletes := 0, 0
	b.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
		if len(opts) == 0 {
			return b.Bucket.Iter(ctx, prefix, f, opts...)
		}
		for _, key := range keys {
			callbacks++
			require.NoError(t, f(key))
		}
		return ctx.Err()
	}
	b.deleteFn = func(context.Context, string) error { deletes++; cancel(); return ctx.Err() }
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(ctx)
	require.Equal(t, 1, deletes)
	require.Equal(t, 2, callbacks)
	require.Zero(t, testutil.ToFloat64(c.success))
	require.Zero(t, testutil.ToFloat64(c.errors.WithLabelValues("list")))
	require.Zero(t, testutil.ToFloat64(c.errors.WithLabelValues("delete")))
}

func TestCleanerServiceCadence(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	entered, release := make(chan time.Time, 2), make(chan struct{})
	b.iter = func(ctx context.Context, _ string, _ func(string) error, _ ...objstore.IterOption) error {
		entered <- time.Now()
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ctx.Err()
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	require.Equal(t, time.Hour, c.sweepInterval)
	c.sweepInterval = 30 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, services.StartAndAwaitRunning(ctx, c))
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c)) })
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("startup sweep missing")
	}
	time.Sleep(2 * c.sweepInterval)
	completed := time.Now()
	close(release)
	select {
	case started := <-entered:
		require.GreaterOrEqual(t, started.Sub(completed), c.sweepInterval)
	case <-ctx.Done():
		t.Fatal("later sweep missing")
	}
	require.NoError(t, services.StopAndAwaitTerminated(ctx, c))
}

func TestCleanerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	c, err := NewCleaner(DefaultCleanerConfig(), b, reg, func() time.Time { return cleanerNow })
	require.NoError(t, err)
	putCleanupKey(t, b, NativeObjectPrefix+"unexpected")
	c.sweep(context.Background())
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
	b.iter = func(context.Context, string, func(string) error, ...objstore.IterOption) error {
		return errors.New("root inaccessible")
	}
	c.now = func() time.Time { return cleanerNow.Add(time.Hour) }
	c.sweep(context.Background())
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
	families, err := reg.Gather()
	require.NoError(t, err)
	var names []string
	for _, family := range families {
		names = append(names, family.GetName())
		if strings.HasSuffix(family.GetName(), "errors_total") {
			require.Len(t, family.Metric, 2)
			for _, metric := range family.Metric {
				require.Len(t, metric.Label, 1)
				require.Equal(t, "operation", metric.Label[0].GetName())
				require.Contains(t, []string{"list", "delete"}, metric.Label[0].GetValue())
			}
		}
	}
	require.ElementsMatch(t, []string{"pyroscope_profile_dump_cleanup_deleted_total", "pyroscope_profile_dump_cleanup_errors_total", "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds"}, names)
}

func TestCleanerConfig(t *testing.T) {
	var cfg CleanerConfig
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlags(flags)
	require.Equal(t, DefaultCleanerConfig(), cfg)
	require.NoError(t, cfg.Validate())
	require.NoError(t, flags.Parse([]string{"-profile-dump.retention=720h"}))
	require.Equal(t, 30*24*time.Hour, cfg.Retention)
	for _, retention := range []time.Duration{0, -time.Hour} {
		require.Error(t, (CleanerConfig{Retention: retention}).Validate())
	}
}
