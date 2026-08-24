package pyroscope

import (
	"context"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/frontend"
	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/v2/pkg/objstore/client"
)

func TestAsyncQueryStorageConfig(t *testing.T) {
	var cfg frontend.AsyncQueriesConfig
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.False(t, cfg.Enabled)
	require.Equal(t, objstoreclient.None, cfg.Storage.Backend)
	require.Equal(t, "./data/async-queries", cfg.Storage.Filesystem.Directory)
	require.NoError(t, fs.Parse([]string{
		"-query-frontend.async-queries.enabled=true",
		"-query-frontend.async-queries.storage.backend=filesystem",
		"-query-frontend.async-queries.storage.filesystem.dir=/tmp/async-query-results",
		"-query-frontend.async-queries.storage.prefix=dedicated",
	}))
	require.True(t, cfg.Enabled)
	require.Equal(t, objstoreclient.Filesystem, cfg.Storage.Backend)
	require.Equal(t, "/tmp/async-query-results", cfg.Storage.Filesystem.Directory)
	require.Equal(t, "dedicated", cfg.Storage.Prefix)
}

func TestAsyncQueryConfigYAML(t *testing.T) {
	cfg := newDefaultConfig()
	decoder := yaml.NewDecoder(strings.NewReader(`
frontend:
  async_queries:
    enabled: true
    storage:
      backend: filesystem
      filesystem:
        dir: /tmp/async-query-results
      prefix: dedicated
`))
	decoder.KnownFields(true)
	require.NoError(t, decoder.Decode(cfg))
	require.True(t, cfg.Frontend.AsyncQueries.Enabled)
	require.Equal(t, objstoreclient.Filesystem, cfg.Frontend.AsyncQueries.Storage.Backend)
	require.Equal(t, "/tmp/async-query-results", cfg.Frontend.AsyncQueries.Storage.Filesystem.Directory)
	require.Equal(t, "dedicated", cfg.Frontend.AsyncQueries.Storage.Prefix)
	require.NoError(t, cfg.Frontend.AsyncQueries.Storage.Validate(log.NewNopLogger()))
}

func TestAsyncQueryStorageDefaults(t *testing.T) {
	cfg := newDefaultConfig()
	require.Equal(t, objstoreclient.None, cfg.Frontend.AsyncQueries.Storage.Backend)
	for _, storage := range []objstoreclient.Config{cfg.Storage.Bucket, cfg.Frontend.AsyncQueries.Storage, cfg.QueryBackend.ResultCache.Storage} {
		require.Equal(t, 10*time.Minute, storage.S3.HTTP.IdleConnTimeout)
		require.Equal(t, 1000, storage.S3.HTTP.MaxIdleConnsPerHost)
		require.Equal(t, 10*time.Minute, storage.GCS.HTTP.IdleConnTimeout)
		require.Equal(t, 1000, storage.GCS.HTTP.MaxIdleConnsPerHost)
	}
}

type asyncQueryTrackingBucket struct {
	phlareobj.Bucket
	iterations chan string
	closed     bool
}

func (b *asyncQueryTrackingBucket) Iter(_ context.Context, dir string, _ func(string) error, _ ...objstore.IterOption) error {
	select {
	case b.iterations <- dir:
	default:
	}
	return nil
}

func (b *asyncQueryTrackingBucket) Close() error {
	b.closed = true
	return nil
}

func TestInitAsyncQueryStoreDefaultStorage(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg := newDefaultConfig()
			cfg.Frontend.AsyncQueries.Enabled = enabled
			bucket := &asyncQueryTrackingBucket{iterations: make(chan string, 1)}
			p := &Pyroscope{
				Cfg:           *cfg,
				logger:        &logger{Logger: log.NewNopLogger()},
				reg:           prometheus.NewRegistry(),
				storageBucket: bucket,
			}
			store, err := p.initAsyncQueryStore()
			require.NoError(t, err)
			if !enabled {
				require.Nil(t, store)
				require.Nil(t, p.asyncQueryStore)
				return
			}
			require.NotNil(t, store)
			require.Same(t, p.asyncQueryStore, store)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, services.StartAndAwaitRunning(ctx, store))
			t.Cleanup(func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				require.NoError(t, services.StopAndAwaitTerminated(stopCtx, store))
				require.False(t, bucket.closed, "the primary bucket must remain open")
			})
			select {
			case dir := <-bucket.iterations:
				require.Equal(t, "async-queries/", dir)
			case <-ctx.Done():
				t.Fatal("async query store did not scan the primary storage bucket")
			}
		})
	}
}
