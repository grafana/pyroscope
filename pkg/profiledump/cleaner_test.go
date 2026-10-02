package profiledump

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
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
	iter     func(context.Context, string, func(string) error) error
	deleteFn func(context.Context, string) error
}

func (b *cleanupTestBucket) Iter(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
	if len(opts) != 0 {
		panic("cleaner must use non-recursive iteration")
	}
	if b.iter != nil {
		return b.iter(ctx, prefix, f)
	}
	return b.Bucket.Iter(ctx, prefix, f)
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
	t.Cleanup(c.closeTraversal)
	return c
}
func cleanupKey(t *testing.T, tenant string, when time.Time) string {
	t.Helper()
	key, _, err := NewObjectKey(tenant, when, FormatPprof)
	require.NoError(t, err)
	return key
}
func putCleanupKey(t *testing.T, b objstore.Bucket, key string) {
	t.Helper()
	// Contents deliberately aren't a readable envelope: timestamps in payloads
	// and profile metadata can never influence cleanup.
	require.NoError(t, b.Upload(context.Background(), key, strings.NewReader(`{"profile_time":"2099-01-01"}`)))
}
func cleanupExists(t *testing.T, b objstore.Bucket, key string, want bool) {
	t.Helper()
	exists, err := b.Exists(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, want, exists, key)
}

func TestCleanerRetentionBoundaryAndPruning(t *testing.T) {
	cfg := DefaultCleanerConfig()
	cutoff := cleanerNow.Add(-cfg.Retention)
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	var listed []string
	b.iter = func(ctx context.Context, prefix string, f func(string) error) error {
		listed = append(listed, prefix)
		return b.Bucket.Iter(ctx, prefix, f)
	}
	times := []time.Time{cutoff.Add(-24 * time.Hour), cutoff.Add(-time.Millisecond), cutoff, cutoff.Add(time.Millisecond), cutoff.Add(24 * time.Hour), cutoff.AddDate(0, 1, 0), cutoff.AddDate(1, 0, 0)}
	var keys []string
	for _, when := range times {
		key := cleanupKey(t, "removed-policy", when)
		keys = append(keys, key)
		putCleanupKey(t, b, key)
	}
	c := newTestCleaner(t, cfg, b)
	// Cleaner has no policy provider: removed/expired/nonexistent policies all
	// share exactly the same retention behavior.
	c.sweep(context.Background())
	for i, key := range keys {
		cleanupExists(t, b, key, i >= 2)
	}
	for _, key := range keys[4:] {
		require.NotContains(t, listed, key[:strings.LastIndex(key, "/")+1])
	}
	require.Equal(t, 2.0, testutil.ToFloat64(c.deleted))
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
	require.Equal(t, 1.0, testutil.ToFloat64(c.passes.WithLabelValues("complete")))
	c.now = func() time.Time { return cleanerNow.Add(time.Millisecond) }
	c.sweep(context.Background())
	cleanupExists(t, b, keys[2], false)
	cleanupExists(t, b, keys[3], true)
}

func TestCleanerStrictDeletionBoundary(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	valid := cleanupKey(t, "tenant", cleanerNow.Add(-8*24*time.Hour))
	day := valid[:strings.LastIndex(valid, "/")+1]
	tenant, _ := EncodeTenant("tenant")
	malformed := []string{
		ObjectPrefix + "bad=/2026/09/10/x.pyrdump",
		ObjectPrefix + tenant + "/2026/13/10/x.pyrdump",
		ObjectPrefix + tenant + "/2026/02/30/x.pyrdump",
		ObjectPrefix + tenant + "/2026/9/10/x.pyrdump",
		ObjectPrefix + tenant + "/1969/09/10/x.pyrdump",
		ObjectPrefix + tenant + "/../09/10/x.pyrdump",
		day + "../outside.pyrdump", day + "unexpected/deeper.pyrdump",
		strings.Replace(valid, "2026/09/10", "2026/09/09", 1),
		strings.Replace(valid, "-pprof.", "-unknown.", 1),
		valid + ".json", strings.Replace(valid, ".pyrdump", ".PYRDUMP", 1),
		day + "0000000000000000000000000!-pprof.pyrdump",
		ObjectPrefix + "unexpected-file", day + "lowercase-ulid.pyrdump",
	}
	unrelated := []string{"tenant/01ABC/profiles.parquet", "other/" + valid, "profile-debug-dumps-other/old.pyrdump"}
	for _, key := range append(append([]string{valid}, malformed...), unrelated...) {
		putCleanupKey(t, b, key)
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(context.Background())
	cleanupExists(t, b, valid, false)
	for _, key := range append(malformed, unrelated...) {
		cleanupExists(t, b, key, true)
	}
	require.Equal(t, float64(len(malformed)), testutil.ToFloat64(c.malformed))
	require.Equal(t, 1.0, testutil.ToFloat64(c.deleted))
}

func TestCleanerUnexpectedProviderKeys(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	outside := cleanupKey(t, "other", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, outside)
	b.iter = func(_ context.Context, _ string, f func(string) error) error {
		return f(outside) // A complete leaf returned while listing the namespace.
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(context.Background())
	cleanupExists(t, b, outside, true)
	require.Equal(t, 1.0, testutil.ToFloat64(c.malformed))
}

func TestCleanerBoundedProgressWithFailures(t *testing.T) {
	cfg := DefaultCleanerConfig()
	cfg.MaxEntries = 7
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	var keys []string
	for i := 0; i < 30; i++ {
		key := cleanupKey(t, "a", cleanerNow.Add(-9*24*time.Hour).Add(time.Duration(i)*time.Millisecond))
		putCleanupKey(t, b, key)
		keys = append(keys, key)
	}
	later := cleanupKey(t, "z", cleanerNow.Add(-9*24*time.Hour))
	putCleanupKey(t, b, later)
	failure := keys[0]
	b.deleteFn = func(ctx context.Context, key string) error {
		if key == failure {
			return errors.New("synthetic failure")
		}
		return b.Bucket.Delete(ctx, key)
	}
	c := newTestCleaner(t, cfg, b)
	passes := 0
	for {
		c.sweep(context.Background())
		passes++
		require.LessOrEqual(t, testutil.ToFloat64(c.deleted), float64(passes*cfg.MaxEntries))
		if len(c.stack) == 0 {
			break
		}
		require.Less(t, passes, 30)
	}
	require.Greater(t, passes, 1)
	cleanupExists(t, b, later, false)
	cleanupExists(t, b, failure, true)
	require.Equal(t, 30.0, testutil.ToFloat64(c.deleted))
	require.Equal(t, 1.0, testutil.ToFloat64(c.errors.WithLabelValues("delete")))
	require.Zero(t, testutil.ToFloat64(c.success))
	b.deleteFn = nil
	for {
		c.sweep(context.Background())
		if len(c.stack) == 0 {
			break
		}
	}
	cleanupExists(t, b, failure, false)
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
}

func TestCleanerListFailureDoesNotStarveOtherTenant(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	failed := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
	later := cleanupKey(t, "z", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, failed)
	putCleanupKey(t, b, later)
	segment, _ := EncodeTenant("a")
	b.iter = func(ctx context.Context, prefix string, f func(string) error) error {
		if prefix == ObjectPrefix+segment+"/" {
			return errors.New("synthetic listing failure")
		}
		return b.Bucket.Iter(ctx, prefix, f)
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(context.Background())
	cleanupExists(t, b, failed, true)
	cleanupExists(t, b, later, false)
	require.Equal(t, 1.0, testutil.ToFloat64(c.errors.WithLabelValues("list")))
	require.Equal(t, 1.0, testutil.ToFloat64(c.passes.WithLabelValues("error")))
	require.Zero(t, testutil.ToFloat64(c.success))
}

func TestCleanerConcurrentNotFound(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	key := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, key)
	b.deleteFn = func(ctx context.Context, key string) error {
		if err := b.Bucket.Delete(ctx, key); err != nil {
			return err
		}
		return b.Bucket.Delete(ctx, key)
	}
	c := newTestCleaner(t, DefaultCleanerConfig(), b)
	c.sweep(context.Background())
	require.Zero(t, testutil.ToFloat64(c.deleted))
	require.Equal(t, 1.0, testutil.ToFloat64(c.missing))
	require.Zero(t, testutil.ToFloat64(c.errors.WithLabelValues("delete")))
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
}

func TestCleanerMultipleServicesShareBucket(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	key := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, key)
	var arrived atomic.Int32
	ready := make(chan struct{})
	b.deleteFn = func(ctx context.Context, key string) error {
		if arrived.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
		return b.Bucket.Delete(ctx, key)
	}
	a, c := newTestCleaner(t, DefaultCleanerConfig(), b), newTestCleaner(t, DefaultCleanerConfig(), b)
	var wg sync.WaitGroup
	for _, cleaner := range []*Cleaner{a, c} {
		wg.Go(func() { cleaner.sweep(context.Background()) })
	}
	wg.Wait()
	cleanupExists(t, b, key, false)
	require.Equal(t, 1.0, testutil.ToFloat64(a.deleted)+testutil.ToFloat64(c.deleted))
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(a.success))
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
}

func TestCleanerCancellationAndTimeout(t *testing.T) {
	for _, operation := range []string{"list", "delete"} {
		for _, timeout := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/timeout=%t", operation, timeout), func(t *testing.T) {
				b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
				putCleanupKey(t, b, cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour)))
				entered := make(chan struct{})
				exited := make(chan struct{})
				var enterOnce, exitOnce sync.Once
				block := func(ctx context.Context) error {
					enterOnce.Do(func() { close(entered) })
					defer exitOnce.Do(func() { close(exited) })
					<-ctx.Done()
					return ctx.Err()
				}
				if operation == "list" {
					b.iter = func(ctx context.Context, _ string, _ func(string) error) error { return block(ctx) }
				} else {
					b.deleteFn = func(ctx context.Context, _ string) error { return block(ctx) }
				}
				cfg := DefaultCleanerConfig()
				if timeout {
					cfg.SweepTimeout = 20 * time.Millisecond
					cfg.SweepInterval = 10 * time.Millisecond
				}
				c := newTestCleaner(t, cfg, b)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, services.StartAndAwaitRunning(ctx, c))
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("didn't enter storage")
				}
				if timeout {
					select {
					case <-exited:
					case <-ctx.Done():
						t.Fatal("didn't time out")
					}
				}
				if timeout {
					require.Eventually(t, func() bool { return testutil.ToFloat64(c.errors.WithLabelValues("timeout")) >= 1 }, time.Second, time.Millisecond)
				}
				require.NoError(t, services.StopAndAwaitTerminated(ctx, c))
				select {
				case <-exited:
				default:
					t.Fatal("storage operation still running")
				}
				require.Zero(t, testutil.ToFloat64(c.success))
				if timeout {
					require.GreaterOrEqual(t, testutil.ToFloat64(c.errors.WithLabelValues("timeout")), 1.0)
				} else {
					require.Zero(t, testutil.ToFloat64(c.errors.WithLabelValues(operation)))
				}
			})
		}
	}
}

func TestCleanerSeparatePrefixOrdering(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	key := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, key)
	// Azure can return a lexically later direct object before common prefixes.
	b.iter = func(ctx context.Context, prefix string, f func(string) error) error {
		if prefix == ObjectPrefix {
			if err := f(ObjectPrefix + "zzzz"); err != nil {
				return err
			}
		}
		return b.Bucket.Iter(ctx, prefix, f)
	}
	cfg := DefaultCleanerConfig()
	cfg.MaxEntries = 1
	c := newTestCleaner(t, cfg, b)
	for range 10 {
		c.sweep(context.Background())
		if len(c.stack) == 0 {
			break
		}
	}
	cleanupExists(t, b, key, false)
	require.Equal(t, 1.0, testutil.ToFloat64(c.malformed))
}

func TestCleanerConfig(t *testing.T) {
	var cfg CleanerConfig
	cfg.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	require.Equal(t, DefaultCleanerConfig(), cfg)
	for _, change := range []func(*CleanerConfig){func(c *CleanerConfig) { c.Retention = 0 }, func(c *CleanerConfig) { c.SweepInterval = -1 }, func(c *CleanerConfig) { c.SweepTimeout = 0 }, func(c *CleanerConfig) { c.MaxEntries = 0 }} {
		bad := cfg
		change(&bad)
		require.Error(t, bad.Validate())
	}
	require.NoError(t, cfg.Validate())
}

func TestCleanerFixedCutoffDuringSweep(t *testing.T) {
	cfg := DefaultCleanerConfig()
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	key := cleanupKey(t, "a", cleanerNow.Add(-cfg.Retention).Add(time.Second))
	putCleanupKey(t, b, key)
	c := newTestCleaner(t, cfg, b)
	b.iter = func(ctx context.Context, prefix string, f func(string) error) error {
		c.now = func() time.Time { return cleanerNow.Add(time.Hour) }
		return b.Bucket.Iter(ctx, prefix, f)
	}
	c.sweep(context.Background())
	cleanupExists(t, b, key, true)
	c.sweep(context.Background())
	cleanupExists(t, b, key, false)
}

func TestCleanerTimeoutSkipsSubtreeAndRetriesNextRound(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	failed := cleanupKey(t, "a", cleanerNow.Add(-8*24*time.Hour))
	later := cleanupKey(t, "z", cleanerNow.Add(-8*24*time.Hour))
	putCleanupKey(t, b, failed)
	putCleanupKey(t, b, later)
	segment, _ := EncodeTenant("a")
	b.iter = func(ctx context.Context, prefix string, f func(string) error) error {
		if prefix == ObjectPrefix+segment+"/" {
			<-ctx.Done()
			return ctx.Err()
		}
		return b.Bucket.Iter(ctx, prefix, f)
	}
	cfg := DefaultCleanerConfig()
	cfg.SweepTimeout = 20 * time.Millisecond
	c := newTestCleaner(t, cfg, b)
	for range 4 {
		c.sweep(context.Background())
		if len(c.stack) == 0 {
			break
		}
	}
	require.Empty(t, c.stack)
	require.Equal(t, 1.0, testutil.ToFloat64(c.errors.WithLabelValues("timeout")))
	cleanupExists(t, b, later, false)
	cleanupExists(t, b, failed, true)
	require.Zero(t, testutil.ToFloat64(c.success))
	b.iter = nil
	c.sweep(context.Background())
	cleanupExists(t, b, failed, false)
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
}

func TestCleanerPartialMetricsHorizonAndListingCancellation(t *testing.T) {
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	cfg := DefaultCleanerConfig()
	cfg.MaxEntries = 5
	old := cleanupKey(t, "a", cleanerNow.Add(-cfg.Retention-time.Hour))
	aging := cleanupKey(t, "a", cleanerNow.Add(-cfg.Retention+time.Hour))
	putCleanupKey(t, b, old)
	putCleanupKey(t, b, aging)
	var listingContexts []context.Context
	b.iter = func(ctx context.Context, prefix string, f func(string) error) error {
		listingContexts = append(listingContexts, ctx)
		return b.Bucket.Iter(ctx, prefix, f)
	}
	c := newTestCleaner(t, cfg, b)
	c.sweep(context.Background())
	cleanupExists(t, b, old, false)
	require.Zero(t, testutil.ToFloat64(c.success))
	require.Equal(t, 1.0, testutil.ToFloat64(c.passes.WithLabelValues("partial")))
	for _, ctx := range listingContexts {
		require.NoError(t, ctx.Err())
	}
	c.now = func() time.Time { return cleanerNow.Add(2 * time.Hour) }
	c.sweep(context.Background())
	cleanupExists(t, b, aging, true) // Frozen traversal horizon, despite advancing wall clock.
	for _, ctx := range listingContexts {
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	}
	require.Equal(t, float64(cleanerNow.Add(2*time.Hour).Unix()), testutil.ToFloat64(c.success))
	for range 3 {
		c.sweep(context.Background())
	}
	cleanupExists(t, b, aging, false)
}

func TestCleanerUTCDateBoundaries(t *testing.T) {
	for _, cutoff := range []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	} {
		t.Run(cutoff.String(), func(t *testing.T) {
			cfg := DefaultCleanerConfig()
			b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
			old := cleanupKey(t, "tenant/../with/slashes", cutoff.Add(-time.Millisecond))
			equal := cleanupKey(t, "tenant/../with/slashes", cutoff)
			putCleanupKey(t, b, old)
			putCleanupKey(t, b, equal)
			c := newTestCleaner(t, cfg, b)
			c.now = func() time.Time { return cutoff.Add(cfg.Retention).In(time.FixedZone("offset", -7*3600)) }
			c.sweep(context.Background())
			cleanupExists(t, b, old, false)
			cleanupExists(t, b, equal, true)
			require.Equal(t, float64(cutoff.Unix()), testutil.ToFloat64(c.successCutoff))
		})
	}
}

func TestCleanerClockRollback(t *testing.T) {
	cfg := DefaultCleanerConfig()
	cfg.MaxEntries = 4
	b := &cleanupTestBucket{Bucket: objstore.NewInMemBucket()}
	key := cleanupKey(t, "a", cleanerNow.Add(-cfg.Retention-time.Hour))
	putCleanupKey(t, b, key)
	c := newTestCleaner(t, cfg, b)
	c.sweep(context.Background())
	require.Zero(t, testutil.ToFloat64(c.successCutoff))
	c.now = func() time.Time { return cleanerNow.Add(-2 * time.Hour) }
	for range 3 {
		c.sweep(context.Background())
		if len(c.stack) == 0 {
			break
		}
	}
	cleanupExists(t, b, key, true)
	require.Equal(t, float64(cleanerNow.Add(-cfg.Retention-2*time.Hour).Unix()), testutil.ToFloat64(c.successCutoff))
}
