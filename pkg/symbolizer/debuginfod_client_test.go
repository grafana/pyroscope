package symbolizer

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestDebuginfodClient(t *testing.T) {
	// Create a test server that returns different responses based on the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buildID := r.URL.Path[len("/buildid/"):]
		buildID = buildID[:len(buildID)-len("/debuginfo")]

		switch buildID {
		case "valid-build-id":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("mock debug info"))
		case "not-found":
			w.WriteHeader(http.StatusNotFound)
		case "server-error":
			w.WriteHeader(http.StatusInternalServerError)
		case "rate-limited":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.Symbolizer.MaxSymbolSizeBytes = 4
		tenantLimits["tenant-limited"] = l
	})

	// Create a client with the test server URL
	metrics := newMetrics(prometheus.NewRegistry())
	client, err := NewDebuginfodClient(log.NewNopLogger(), server.URL, metrics, limits)
	require.NoError(t, err)

	// Test cases
	tests := []struct {
		name          string
		buildID       string
		tenantID      string
		expectedError bool
		expectedData  string
		errorCheck    func(error) bool
	}{
		{
			name:          "valid build ID",
			buildID:       "valid-build-id",
			expectedError: false,
			expectedData:  "mock debug info",
		},
		{
			name:          "not found",
			buildID:       "not-found",
			expectedError: true,
			errorCheck: func(err error) bool {
				var notFoundErr buildIDNotFoundError
				return errors.As(err, &notFoundErr)
			},
		},
		{
			name:          "server error",
			buildID:       "server-error",
			expectedError: true,
			errorCheck: func(err error) bool {
				return err != nil && err.Error() != "" &&
					(err.Error() == "HTTP error 500" ||
						err.Error() == "failed to fetch debuginfo after 3 attempts: HTTP error 500")
			},
		},
		{
			name:          "rate limited",
			buildID:       "rate-limited",
			expectedError: true,
			errorCheck: func(err error) bool {
				return err != nil && err.Error() != "" &&
					(err.Error() == "HTTP error 429" ||
						err.Error() == "failed to fetch debuginfo after 3 attempts: HTTP error 429")
			},
		},
		{
			name:          "invalid build ID",
			buildID:       "invalid/build/id",
			expectedError: true,
			errorCheck: func(err error) bool {
				return isInvalidBuildIDError(err)
			},
		},
		{
			name:          "size limit",
			buildID:       "valid-build-id",
			tenantID:      "tenant-limited",
			expectedError: true,
			errorCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "symbol size exceeds maximum allowed size of 4 bytes")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Fetch debug info
			tenantID := "tenant"
			if tc.tenantID != "" {
				tenantID = tc.tenantID
			}
			ctx := tenant.InjectTenantID(context.Background(), tenantID)
			reader, err := client.FetchDebuginfo(ctx, tc.buildID)

			// Check error
			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorCheck != nil {
					assert.True(t, tc.errorCheck(err), "Error type check failed: %v", err)
				}
				return
			}

			// Check success case
			require.NoError(t, err)
			defer reader.Close()

			// Read the data
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedData, string(data))
		})
	}
}

func TestDebuginfodClientSingleflight(t *testing.T) {
	// Create a test server that sleeps to simulate a slow response
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock debug info"))
	}))
	defer server.Close()

	// Create a client with the test server URL
	metrics := newMetrics(prometheus.NewRegistry())
	client, err := NewDebuginfodClient(log.NewNopLogger(), server.URL, metrics, validation.MockDefaultOverrides())
	require.NoError(t, err)

	// Make concurrent requests with the same build ID
	buildID := "singleflight-test-id"
	ctx := tenant.InjectTenantID(context.Background(), "tenant")

	// Channel to synchronize goroutines
	done := make(chan struct{})
	results := make(chan []byte, 3)
	errors := make(chan error, 3)

	// Start 3 concurrent requests
	for i := 0; i < 3; i++ {
		go func() {
			reader, err := client.FetchDebuginfo(ctx, buildID)
			if err != nil {
				errors <- err
				done <- struct{}{}
				return
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				errors <- err
			} else {
				results <- data
			}
			done <- struct{}{}
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Check results
	close(results)
	close(errors)

	// Should have no errors
	for err := range errors {
		t.Errorf("Unexpected error: %v", err)
	}

	// All results should be the same
	var data []byte
	for result := range results {
		if data == nil {
			data = result
		} else {
			assert.Equal(t, data, result)
		}
	}

	// Should have made only one HTTP request
	assert.Equal(t, 1, requestCount, "Expected only one HTTP request")
}

func TestDebuginfodClientCanceledCallerDoesNotWaitForFetch(t *testing.T) {
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock debug info"))
	}))
	defer server.Close()

	metrics := newMetrics(prometheus.NewRegistry())
	client, err := NewDebuginfodClient(log.NewNopLogger(), server.URL, metrics, validation.MockDefaultOverrides())
	require.NoError(t, err)

	buildID := "cancel-wait-test-id"

	// A waiter with a live context stays on the shared fetch until it finishes.
	type fetchResult struct {
		data []byte
		err  error
	}
	waiterCh := make(chan fetchResult, 1)
	go func() {
		reader, err := client.FetchDebuginfo(tenant.InjectTenantID(context.Background(), "tenant"), buildID)
		if err != nil {
			waiterCh <- fetchResult{err: err}
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		waiterCh <- fetchResult{data: data, err: err}
	}()

	<-requestStarted

	// A canceled caller joining the same in-flight fetch returns its own
	// context error immediately instead of blocking until the fetch finishes.
	canceledCtx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), "tenant"))
	cancel()
	_, err = client.FetchDebuginfo(canceledCtx, buildID)
	require.ErrorIs(t, err, context.Canceled)

	close(release)
	res := <-waiterCh
	require.NoError(t, res.err)
	require.Equal(t, "mock debug info", string(res.data))
}

func TestSanitizeBuildID(t *testing.T) {
	tests := []struct {
		name        string
		buildID     string
		expected    string
		expectError bool
	}{
		{
			name:        "valid build ID",
			buildID:     "abcdef1234567890",
			expected:    "abcdef1234567890",
			expectError: false,
		},
		{
			name:        "valid build ID with dashes and underscores",
			buildID:     "abcdef-1234_7890",
			expected:    "abcdef-1234_7890",
			expectError: false,
		},
		{
			name:        "invalid build ID with slashes",
			buildID:     "abcdef/1234",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid build ID with spaces",
			buildID:     "abcdef 1234",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid build ID with special characters",
			buildID:     "abcdef#1234",
			expected:    "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sanitizeBuildID(tc.buildID)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "invalid build ID",
			err:      invalidBuildIDError{buildID: "invalid"},
			expected: false,
		},
		{
			name:     "build ID not found",
			err:      buildIDNotFoundError{buildID: "not-found"},
			expected: false,
		},
		{
			name:     "HTTP 404",
			err:      httpStatusError{statusCode: http.StatusNotFound},
			expected: false,
		},
		{
			name:     "HTTP 429",
			err:      httpStatusError{statusCode: http.StatusTooManyRequests},
			expected: true,
		},
		{
			name:     "HTTP 500",
			err:      httpStatusError{statusCode: http.StatusInternalServerError},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryableError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDebuginfodClientNotFoundCache(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		buildID := r.URL.Path[len("/buildid/"):]
		buildID = buildID[:len(buildID)-len("/debuginfo")]
		if buildID == "not-found-cached" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock debug info"))
	}))
	defer server.Close()

	client, err := NewDebuginfodClientWithConfig(log.NewNopLogger(), DebuginfodClientConfig{
		BaseURL:               server.URL,
		NotFoundCacheMaxItems: 100,
		NotFoundCacheTTL:      10 * time.Second,
	}, newMetrics(nil), validation.MockDefaultOverrides())
	require.NoError(t, err)

	ctx := tenant.InjectTenantID(context.Background(), "tenant")
	buildID := "not-found-cached"

	// First request should hit the server and get a 404
	reader, err := client.FetchDebuginfo(ctx, buildID)
	assert.Error(t, err)
	assert.Nil(t, reader)

	var notFoundErr buildIDNotFoundError
	assert.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, 1, requestCount)

	client.notFoundCache.Wait()

	// Second request should get 404 from cache without hitting server
	reader, err = client.FetchDebuginfo(ctx, buildID)
	assert.Error(t, err)
	assert.Nil(t, reader)
	assert.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, 1, requestCount)

	// Third request should hit the server
	reader, err = client.FetchDebuginfo(ctx, "valid-id")
	assert.NoError(t, err)
	require.NotNil(t, reader)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	reader.Close()
	assert.Equal(t, "mock debug info", string(data))

	assert.Equal(t, 2, requestCount)
}

func TestIsRetryableError_ConnectionFailures(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"connection refused", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, true},
		{"connection reset", &net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, true},
		{"broken pipe", &net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EPIPE)}, true},
		{"eof", io.EOF, false},
		{"unexpected eof", io.ErrUnexpectedEOF, false},
		{"dns error", &net.DNSError{Err: "no such host", Name: "example.invalid"}, true},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"canceled", context.Canceled, false},
		{"http not found", httpStatusError{statusCode: http.StatusNotFound}, false},
		{"http rate limited", httpStatusError{statusCode: http.StatusTooManyRequests}, true},
		{"http server error", httpStatusError{statusCode: http.StatusInternalServerError}, true},
		{"invalid build id", invalidBuildIDError{buildID: "!"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, isRetryableError(tt.err))
		})
	}
}

// countingTransport counts round trips, including ones that fail to connect.
type countingTransport struct {
	inner http.RoundTripper
	calls atomic.Int32
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.inner.RoundTrip(req)
}

func TestFetchDebuginfo_RetriesRefusedConnections(t *testing.T) {
	// A closed listener's address yields a real ECONNREFUSED on every dial.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverURL := "http://" + l.Addr().String()
	require.NoError(t, l.Close())

	transport := &countingTransport{inner: http.DefaultTransport}
	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {})
	client, err := NewDebuginfodClientWithConfig(log.NewNopLogger(), DebuginfodClientConfig{
		BaseURL:    serverURL,
		HTTPClient: &http.Client{Transport: transport},
		BackoffConfig: backoff.Config{
			MinBackoff: time.Millisecond,
			MaxBackoff: 2 * time.Millisecond,
			MaxRetries: 3,
		},
		NotFoundCacheMaxItems: 10,
		NotFoundCacheTTL:      time.Minute,
	}, newMetrics(prometheus.NewRegistry()), limits)
	require.NoError(t, err)

	ctx := tenant.InjectTenantID(context.Background(), "test-tenant")
	_, err = client.FetchDebuginfo(ctx, "deadbeef")
	require.Error(t, err)
	assert.ErrorContains(t, err, "after 3 attempts")
	assert.Equal(t, int32(3), transport.calls.Load(),
		"refused connections must be retried up to the backoff budget")
}

func TestFetchDebuginfo_NotFoundCacheExpires(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {})
	client, err := NewDebuginfodClientWithConfig(log.NewNopLogger(), DebuginfodClientConfig{
		BaseURL: server.URL,
		BackoffConfig: backoff.Config{
			MinBackoff: time.Millisecond,
			MaxBackoff: 2 * time.Millisecond,
			MaxRetries: 1,
		},
		NotFoundCacheMaxItems: 1000,
		NotFoundCacheTTL:      20 * time.Millisecond,
	}, newMetrics(prometheus.NewRegistry()), limits)
	require.NoError(t, err)

	ctx := tenant.InjectTenantID(context.Background(), "test-tenant")
	_, err = client.FetchDebuginfo(ctx, "cafebabe")
	require.Error(t, err)
	require.Equal(t, int32(1), calls.Load())

	_, err = client.FetchDebuginfo(ctx, "cafebabe")
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "a 404 within the TTL must be served from the cache")

	time.Sleep(100 * time.Millisecond)

	_, err = client.FetchDebuginfo(ctx, "cafebabe")
	require.Error(t, err)
	assert.Equal(t, int32(2), calls.Load(), "an expired 404 entry must be fetched again")
}

func TestFetchDebuginfo_CircuitBreakerFailsFast(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverURL := "http://" + l.Addr().String()
	require.NoError(t, l.Close())

	transport := &countingTransport{inner: http.DefaultTransport}
	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {})
	client, err := NewDebuginfodClientWithConfig(log.NewNopLogger(), DebuginfodClientConfig{
		BaseURL:    serverURL,
		HTTPClient: &http.Client{Transport: transport},
		BackoffConfig: backoff.Config{
			MinBackoff: time.Millisecond,
			MaxBackoff: 2 * time.Millisecond,
			MaxRetries: 1,
		},
		NotFoundCacheMaxItems:   1000,
		NotFoundCacheTTL:        time.Minute,
		BreakerFailureThreshold: 2,
		BreakerOpenDuration:     time.Minute,
	}, newMetrics(prometheus.NewRegistry()), limits)
	require.NoError(t, err)

	ctx := tenant.InjectTenantID(context.Background(), "test-tenant")

	// Two distinct build IDs fail at the network level and trip the breaker.
	_, err = client.FetchDebuginfo(ctx, "buildid1")
	require.Error(t, err)
	_, err = client.FetchDebuginfo(ctx, "buildid2")
	require.Error(t, err)
	dialsAfterTrip := transport.calls.Load()
	require.Greater(t, dialsAfterTrip, int32(0))

	// The third build ID fails fast without touching the network.
	_, err = client.FetchDebuginfo(ctx, "buildid3")
	require.Error(t, err)
	var unavailable upstreamUnavailableError
	assert.ErrorAs(t, err, &unavailable)
	assert.Equal(t, dialsAfterTrip, transport.calls.Load(),
		"an open breaker must not dial the upstream")
}

// highWaterTransport tracks the maximum number of concurrent in-flight
// round trips.
type highWaterTransport struct {
	inner    http.RoundTripper
	inflight atomic.Int32
	max      atomic.Int32
}

func (h *highWaterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cur := h.inflight.Add(1)
	defer h.inflight.Add(-1)
	for {
		observed := h.max.Load()
		if cur <= observed || h.max.CompareAndSwap(observed, cur) {
			break
		}
	}
	return h.inner.RoundTrip(req)
}

func TestFetchDebuginfo_BoundsConcurrentUpstreamFetches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("debug info"))
	}))
	defer server.Close()

	transport := &highWaterTransport{inner: http.DefaultTransport}
	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {})
	client, err := NewDebuginfodClientWithConfig(log.NewNopLogger(), DebuginfodClientConfig{
		BaseURL:               server.URL,
		HTTPClient:            &http.Client{Transport: transport},
		BackoffConfig:         backoff.Config{MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, MaxRetries: 1},
		NotFoundCacheMaxItems: 1000,
		NotFoundCacheTTL:      time.Minute,
		MaxConcurrentFetches:  2,
	}, newMetrics(prometheus.NewRegistry()), limits)
	require.NoError(t, err)

	ctx := tenant.InjectTenantID(context.Background(), "test-tenant")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buildID := "buildid" + strconv.Itoa(i)
			r, err := client.FetchDebuginfo(ctx, buildID)
			if err == nil {
				_ = r.Close()
			}
		}()
	}
	wg.Wait()
	assert.LessOrEqual(t, transport.max.Load(), int32(2),
		"in-flight upstream fetches must stay within MaxConcurrentFetches")
}
