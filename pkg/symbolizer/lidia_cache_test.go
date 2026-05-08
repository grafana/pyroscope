package symbolizer

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

func TestLidiaCache_HitOnRepeatedBuildID(t *testing.T) {
	const buildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"
	lidiaData, err := extractGzipFile(t, "testdata/test_lidia_file.gz")
	require.NoError(t, err)

	s, _, mockBucket := newSymbolizerTest(t, &symbolizerInputs{LidiaCacheSizeBytes: 128 << 20})

	// One Get for the cache fill; subsequent calls must be served from cache.
	mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", buildID)).
		Return(io.NopCloser(bytes.NewReader(lidiaData)), nil).Once()

	ctx := tenant.InjectTenantID(context.Background(), "tenant")

	for i := 0; i < 5; i++ {
		req := &request{
			buildID:    buildID,
			binaryName: "test-binary",
			locations:  []*location{{address: 0x1b743d6}},
		}
		s.symbolize(ctx, req)
		require.NotEmpty(t, req.locations[0].lines, "iteration %d returned no symbols", i)
	}

	mockBucket.AssertExpectations(t)
}

func TestLidiaCache_ConcurrentMissesShareOneFetch(t *testing.T) {
	const buildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"
	lidiaData, err := extractGzipFile(t, "testdata/test_lidia_file.gz")
	require.NoError(t, err)

	s, _, mockBucket := newSymbolizerTest(t, &symbolizerInputs{LidiaCacheSizeBytes: 128 << 20})

	var gets atomic.Int64
	// Hold the first Get briefly so concurrent callers all observe the cache
	// miss and queue behind singleflight, rather than serializing naturally.
	mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", buildID)).
		Return(func(_ context.Context, _ string) (io.ReadCloser, error) {
			gets.Add(1)
			time.Sleep(10 * time.Millisecond)
			return io.NopCloser(bytes.NewReader(lidiaData)), nil
		}, nil)

	ctx := tenant.InjectTenantID(context.Background(), "tenant")

	const concurrency = 16
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			req := &request{
				buildID:    buildID,
				binaryName: "test-binary",
				locations:  []*location{{address: 0x1b743d6}},
			}
			s.symbolize(ctx, req)
			require.NotEmpty(t, req.locations[0].lines)
		}()
	}
	wg.Wait()

	require.Equal(t, int64(1), gets.Load(), "singleflight should coalesce all concurrent misses to one fetch")
}
