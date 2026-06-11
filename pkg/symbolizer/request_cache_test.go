package symbolizer

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

const testLidiaBuildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"

func loadTestLidiaData(t *testing.T) []byte {
	t.Helper()
	lidiaData, err := extractGzipFile(t, "testdata/test_lidia_file.gz")
	require.NoError(t, err)
	require.NotEmpty(t, lidiaData)
	return lidiaData
}

func newLidiaRequest(address uint64) *request {
	return &request{
		buildID:    testLidiaBuildID,
		binaryName: "test-binary",
		locations:  []*location{{address: address}},
	}
}

func TestRequestCache_DedupsObjectStoreReads(t *testing.T) {
	lidiaData := loadTestLidiaData(t)
	sym, _, mockBucket := newSymbolizerTest(t, nil)

	// A single Get must serve all symbolize calls within the request.
	mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", testLidiaBuildID)).
		Return(io.NopCloser(bytes.NewReader(lidiaData)), nil).Once()

	ctx := WithRequestCache(tenant.InjectTenantID(context.Background(), "tenant"))
	for i := 0; i < 3; i++ {
		req := newLidiaRequest(0x1b743d6)
		sym.symbolize(ctx, req)
		require.NotEmpty(t, req.locations[0].lines)
	}

	mockBucket.AssertExpectations(t)
}

func TestRequestCache_KeysAreTenantScoped(t *testing.T) {
	lidiaData := loadTestLidiaData(t)
	sym, _, mockBucket := newSymbolizerTest(t, nil)

	mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant-a", testLidiaBuildID)).
		Return(io.NopCloser(bytes.NewReader(lidiaData)), nil).Once()
	mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant-b", testLidiaBuildID)).
		Return(io.NopCloser(bytes.NewReader(lidiaData)), nil).Once()

	// Both tenants share the same request cache: tenant isolation must come
	// from the cache key, not from separate caches.
	ctx := WithRequestCache(context.Background())
	for _, tenantID := range []string{"tenant-a", "tenant-b"} {
		req := newLidiaRequest(0x1b743d6)
		sym.symbolize(tenant.InjectTenantID(ctx, tenantID), req)
		require.NotEmpty(t, req.locations[0].lines)
	}

	mockBucket.AssertExpectations(t)
}

func TestRequestCache_ConcurrentCallsShareOneFetch(t *testing.T) {
	c := &requestCache{items: make(map[string][]byte)}
	m := newMetrics(prometheus.NewRegistry())

	var mu sync.Mutex
	fetches := 0
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			b, err := c.getOrFetch("tenant/build-id", m, func() ([]byte, error) {
				mu.Lock()
				fetches++
				mu.Unlock()
				return []byte("lidia-bytes"), nil
			})
			require.NoError(t, err)
			require.Equal(t, []byte("lidia-bytes"), b)
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, 1, fetches, "concurrent calls for the same key must share a single fetch")
}

func TestRequestCache_ErrorsAreNotCached(t *testing.T) {
	c := &requestCache{items: make(map[string][]byte)}
	m := newMetrics(prometheus.NewRegistry())

	fetches := 0
	_, err := c.getOrFetch("tenant/build-id", m, func() ([]byte, error) {
		fetches++
		return nil, context.Canceled
	})
	require.Error(t, err)

	b, err := c.getOrFetch("tenant/build-id", m, func() ([]byte, error) {
		fetches++
		return []byte("lidia-bytes"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []byte("lidia-bytes"), b)
	require.Equal(t, 2, fetches, "a failed fetch must not poison subsequent calls")
}

func TestRequestCache_CapFallsThroughWithoutCaching(t *testing.T) {
	c := &requestCache{items: make(map[string][]byte), held: maxRequestCacheBytes}
	m := newMetrics(prometheus.NewRegistry())

	fetches := 0
	for i := 0; i < 2; i++ {
		b, err := c.getOrFetch("tenant/build-id", m, func() ([]byte, error) {
			fetches++
			return []byte("lidia-bytes"), nil
		})
		require.NoError(t, err)
		require.Equal(t, []byte("lidia-bytes"), b)
	}

	require.Equal(t, 2, fetches, "entries above the cap must be served fetch-through")
	require.Empty(t, c.items)
}
