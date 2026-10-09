package querybackend

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestConfig_ResultCacheExecutionDelay(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		want    time.Duration
		invalid bool
	}{
		{name: "default", want: 15 * time.Millisecond},
		{name: "zero", args: []string{"-query-backend.result-cache.execution-delay=0s"}},
		{name: "custom", args: []string{"-query-backend.result-cache.execution-delay=50ms"}, want: 50 * time.Millisecond},
		{name: "negative", args: []string{"-query-backend.result-cache.execution-delay=-1ms"}, want: -time.Millisecond, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
			cfg.RegisterFlags(fs)
			require.NoError(t, fs.Parse(tt.args))
			require.Equal(t, tt.want, cfg.ResultCache.ExecutionDelay)
			if tt.invalid {
				require.EqualError(t, cfg.Validate(), "query-backend.result-cache.execution-delay must not be negative")
			} else {
				require.NoError(t, cfg.Validate())
			}
		})
	}
}

func TestConfig_ResultCache(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	cfg.RegisterFlags(fs)
	var limits validation.Limits
	limits.RegisterFlags(fs)
	require.False(t, limits.ResultCacheEnabled)
	require.Empty(t, cfg.ResultCache.Storage.Backend)
	require.NoError(t, fs.Parse([]string{
		"-query-backend.result-cache.enabled=true",
		"-query-backend.result-cache.storage.backend=s3",
		"-query-backend.result-cache.storage.s3.bucket-name=cache",
	}))
	require.True(t, limits.ResultCacheEnabled)
	require.Equal(t, "s3", cfg.ResultCache.Storage.Backend)
	require.Equal(t, "cache", cfg.ResultCache.Storage.S3.BucketName)
}

func TestResultCacheDisabled(t *testing.T) {
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	q, err := New(Config{}, nil, nil, nil, nil, bucket, resultCacheOverrides{enabled: false})
	require.NoError(t, err)
	req, _ := cacheTestRequest()
	require.False(t, q.resultCacheEligible(req))
}

func TestBlockCacheConfiguredExecutionDelay(t *testing.T) {
	for _, delay := range []time.Duration{0, 50 * time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			bucket := &delayedCacheBucket{
				Bucket:  objstore.NewBucket(thanobjstore.NewInMemBucket()),
				release: make(chan struct{}),
			}
			var elapsed time.Duration
			start := time.Now()
			q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				elapsed = time.Since(start)
				return cacheTestResponse(), nil
			}))
			q.resultCacheExecutionDelay = delay
			req, _ := cacheTestRequest()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := q.executeBlocks(ctx, req)
			require.NoError(t, err)
			require.GreaterOrEqual(t, elapsed, delay)
		})
	}
}
