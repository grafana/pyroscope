package profiledump

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
)

// Exercise the pinned Thanos/MinIO producer, including a buffered page and an
// outstanding HTTP request. A callback error used to strand its terminal send.
func TestCleanerS3Cancellation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%t", pending), func(t *testing.T) {
			baseline := goleak.IgnoreCurrent()
			defer goleak.VerifyNone(t, baseline)
			entered := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				if pending {
					<-r.Context().Done()
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				_, _ = fmt.Fprint(w, `<ListBucketResult><Name>captures</Name><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken>`)
				for i := range 1000 {
					_, _ = fmt.Fprintf(w, "<Contents><Key>profile-debug-dumps/unexpected-%04d</Key><Size>1</Size></Contents>", i)
				}
				_, _ = fmt.Fprint(w, `</ListBucketResult>`)
			}))
			defer srv.Close()
			var cfg s3.Config
			cfg.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
			cfg.Endpoint = strings.TrimPrefix(srv.URL, "http://")
			cfg.BucketName, cfg.Region, cfg.Insecure = "captures", "us-east-1", true
			cfg.AccessKeyID = "local-test-key"
			require.NoError(t, cfg.SecretAccessKey.Set("local-test-secret"))
			cfg.BucketLookupType = s3.PathStyleLookup
			cfg.HTTP.Transport = srv.Client().Transport
			bucket, err := s3.NewBucketClient(cfg, "test", log.NewNopLogger())
			require.NoError(t, err)
			defer func() { require.NoError(t, bucket.Close()) }()
			settings := DefaultCleanerConfig()
			settings.MaxEntries = 1
			cleaner := newTestCleaner(t, settings, bucket)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			passDone := make(chan struct{})
			go func() { defer close(passDone); cleaner.sweep(ctx) }()
			<-entered
			if !pending {
				<-passDone
				require.Equal(t, 1.0, testutil.ToFloat64(cleaner.passes.WithLabelValues("partial")))
				require.Zero(t, testutil.ToFloat64(cleaner.success))
			}
			cancel()
			<-passDone
			cleaner.closeTraversal()
			cleaner.closeTraversal()
		})
	}
}

func TestCleanerStreamingBoundsAndProgress(t *testing.T) {
	// This provider streams a large namespace without retaining a key collection.
	// Every malformed entry remains present. Rescanning would never finish.
	b := &cleanupTestBucket{}
	const entries = 100003
	var calls, callbacks, active atomic.Int64
	b.iter = func(ctx context.Context, _ string, f func(string) error) error {
		calls.Add(1)
		active.Add(1)
		defer active.Add(-1)
		for i := range entries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			callbacks.Add(1)
			if err := f(fmt.Sprintf("%sunexpected-%d", ObjectPrefix, i)); err != nil {
				return err
			}
		}
		return nil
	}
	cfg := DefaultCleanerConfig()
	cfg.MaxEntries = 97
	c := newTestCleaner(t, cfg, b)
	passes := 0
	for {
		before := testutil.ToFloat64(c.malformed)
		c.sweep(context.Background())
		passes++
		require.LessOrEqual(t, testutil.ToFloat64(c.malformed)-before, float64(cfg.MaxEntries))
		require.LessOrEqual(t, len(c.stack), 5)
		require.LessOrEqual(t, active.Load(), int64(1))
		// At most one delivered entry can be parked inside the callback.
		require.LessOrEqual(t, callbacks.Load()-int64(testutil.ToFloat64(c.malformed)), int64(1))
		if len(c.stack) == 0 {
			break
		}
		require.LessOrEqual(t, passes, entries/cfg.MaxEntries+1)
	}
	require.EqualValues(t, 1, calls.Load())
	require.EqualValues(t, entries, testutil.ToFloat64(c.malformed))
	require.Zero(t, active.Load())
}

func TestCleanerPendingFetchSurvivesPartialPass(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	b := &cleanupTestBucket{iter: func(_ context.Context, _ string, f func(string) error) error {
		close(entered)
		<-release
		return f(ObjectPrefix + "invalid")
	}}
	it := newCleanupIterator(context.Background(), b, ObjectPrefix, time.Minute)
	defer it.stop()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, _, err := it.next(ctx); result <- err }()
	<-entered
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	close(release)
	key, finished, err := it.next(context.Background())
	require.NoError(t, err)
	require.False(t, finished)
	require.Equal(t, ObjectPrefix+"invalid", key)
	_, finished, err = it.next(context.Background())
	require.NoError(t, err)
	require.True(t, finished)
}
