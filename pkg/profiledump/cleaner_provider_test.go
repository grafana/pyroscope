package profiledump

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.uber.org/goleak"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
)

// Returning callback errors can strand the pinned Thanos/MinIO producer with a buffered page.
func TestCleanerS3Cancellation(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, baseline)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<ListBucketResult><Name>captures</Name><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken>`)
		for i := range 1000 {
			_, _ = fmt.Fprintf(w, "<Contents><Key>%sunexpected-%04d</Key><Size>1</Size></Contents>", ObjectPrefix, i)
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
			cancel()
			return f(key)
		}, opts...)
	}
	cleaner := newTestCleaner(t, settings, wrapped)
	passDone := make(chan struct{})
	go func() {
		defer close(passDone)
		cleaner.sweep(ctx)
	}()
	await(t, passDone)
	require.Positive(t, callbacks)
	require.Zero(t, testutil.ToFloat64(cleaner.success))
}
