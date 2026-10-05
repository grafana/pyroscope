package profiledump

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	cosprovider "github.com/thanos-io/objstore/providers/cos"
	"go.uber.org/goleak"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
)

// Exercise the pinned Thanos/MinIO producer, including a buffered page and an
// outstanding HTTP request. Returning callback errors can strand the terminal send.
func TestCleanerS3Cancellation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%t", pending), func(t *testing.T) {
			baseline := goleak.IgnoreCurrent()
			defer goleak.VerifyNone(t, baseline)
			entered := make(chan struct{})
			var once sync.Once
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				once.Do(func() { close(entered) })
				if pending {
					<-r.Context().Done()
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				_, _ = fmt.Fprint(w, `<ListBucketResult><Name>captures</Name><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken>`)
				for i := range 1000 {
					_, _ = fmt.Fprintf(w, "<Contents><Key>profile-debug-dumps/native/unexpected-%04d</Key><Size>1</Size></Contents>", i)
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

			wrapped := &cleanupTestBucket{Bucket: bucket}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			callbacks := 0
			wrapped.iter = func(ctx context.Context, prefix string, f func(string) error, opts ...objstore.IterOption) error {
				return bucket.Iter(ctx, prefix, func(key string) error {
					callbacks++
					if !pending {
						cancel()
					}
					return f(key)
				}, opts...)
			}
			cleaner := newTestCleaner(t, settings, wrapped)
			passDone := make(chan struct{})
			go func() { defer close(passDone); cleaner.sweep(ctx) }()
			<-entered
			if pending {
				cancel()
			}
			<-passDone
			if !pending {
				require.Positive(t, callbacks)
			}
			require.Zero(t, testutil.ToFloat64(cleaner.success))
			require.Zero(t, testutil.ToFloat64(cleaner.errors.WithLabelValues("list")))
			require.Zero(t, testutil.ToFloat64(cleaner.deleted))
		})
	}
}

// COS classifies every HTTP 404 as object-not-found, including NoSuchBucket.
// Use the actual provider with an in-process transport to distinguish a failed
// root listing from a successful empty namespace without live storage access.
func TestCleanerCOSMissingBucket(t *testing.T) {
	transport := &cleanupCOSTransport{missingBucket: true}
	cfg := cosprovider.DefaultConfig
	cfg.Endpoint = "https://cleanup-fixture.invalid"
	cfg.SecretId = "local-fixture-id"
	cfg.SecretKey = "local-fixture-key"
	bucket, err := cosprovider.NewBucketWithConfig(log.NewNopLogger(), cfg, "test", func(http.RoundTripper) http.RoundTripper { return transport })
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })

	err = bucket.Iter(context.Background(), NativeObjectPrefix, func(string) error {
		t.Fatal("unexpected object")
		return nil
	})
	require.Error(t, err)
	require.True(t, bucket.IsObjNotFoundErr(err))

	c := newTestCleaner(t, DefaultCleanerConfig(), bucket)
	c.sweep(context.Background())
	require.Equal(t, 1.0, testutil.ToFloat64(c.errors.WithLabelValues("list")))
	require.Zero(t, testutil.ToFloat64(c.success))

	transport.missingBucket = false
	c.sweep(context.Background())
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))

	transport.missingBucket = true
	c.now = func() time.Time { return cleanerNow.Add(time.Hour) }
	c.sweep(context.Background())
	require.Equal(t, 2.0, testutil.ToFloat64(c.errors.WithLabelValues("list")))
	require.Equal(t, float64(cleanerNow.Unix()), testutil.ToFloat64(c.success))
	require.Zero(t, testutil.ToFloat64(c.deleted))
	require.Zero(t, testutil.ToFloat64(c.errors.WithLabelValues("delete")))
}

type cleanupCOSTransport struct {
	missingBucket bool
}

func (t *cleanupCOSTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	status := http.StatusOK
	body := `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
	if t.missingBucket {
		status = http.StatusNotFound
		body = `<Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist.</Message><Resource>/</Resource><RequestId>local-test</RequestId></Error>`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}
