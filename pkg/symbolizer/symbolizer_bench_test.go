package symbolizer

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"sync/atomic"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mocksymbolizer"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// Microbenchmark: same buildID symbolized repeatedly. Synthetic best case,
// production numbers will be smaller. Sub-benchmarks A/B cache off vs on.
func BenchmarkSymbolizeRepeatedBuildID(b *testing.B) {
	cases := []struct {
		name      string
		cacheSize int64
	}{
		{"off", 0},
		{"on_512MiB", 512 << 20},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			runRepeatedBuildIDBench(b, tc.cacheSize)
		})
	}
}

func runRepeatedBuildIDBench(b *testing.B, cacheSize int64) {
	const buildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"
	const addr = 0x1b743d6

	lidiaData := mustReadGzip(b, "testdata/test_lidia_file.gz")

	bucket := mockobjstore.NewMockBucket(b)
	// Match every Get call; return a fresh reader each time so the benchmark
	// reflects the io.ReadAll cost per call accurately.
	var bucketGets atomic.Int64
	bucket.On("Get", mock.Anything, lidiaObjectPath("tenant", buildID)).
		Return(func(_ context.Context, _ string) (io.ReadCloser, error) {
			bucketGets.Add(1)
			return io.NopCloser(bytes.NewReader(lidiaData)), nil
		}, nil)

	s, err := New(
		log.NewNopLogger(),
		Config{
			MaxDebuginfodConcurrency: 1,
			LidiaCacheSizeBytes:      cacheSize,
		},
		prometheus.NewRegistry(),
		bucket,
		validation.MockDefaultOverrides(),
	)
	require.NoError(b, err)
	s.client = mocksymbolizer.NewMockDebuginfodClient(b)
	b.Cleanup(s.Close)

	ctx := tenant.InjectTenantID(context.Background(), "tenant")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &request{
			buildID:    buildID,
			binaryName: "test-binary",
			locations:  []*location{{address: addr}},
		}
		s.symbolize(ctx, req)
	}
	b.StopTimer()

	b.ReportMetric(float64(bucketGets.Load())/float64(b.N), "bucket_gets/op")
}

func mustReadGzip(b *testing.B, path string) []byte {
	b.Helper()
	f, err := os.Open(path)
	require.NoError(b, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(b, err)
	defer gz.Close()
	data, err := io.ReadAll(gz)
	require.NoError(b, err)
	return data
}
