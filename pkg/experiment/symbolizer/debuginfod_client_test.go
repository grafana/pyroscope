package symbolizer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Create a client with the test server URL
	metrics := newMetrics(prometheus.NewRegistry())
	client, err := NewDebuginfodClient(log.NewNopLogger(), server.URL, metrics)
	require.NoError(t, err)

	// Test cases
	tests := []struct {
		name          string
		buildID       string
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Fetch debug info
			reader, err := client.FetchDebuginfo(context.Background(), tc.buildID)

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
	client, err := NewDebuginfodClient(log.NewNopLogger(), server.URL, metrics)
	require.NoError(t, err)

	// Make concurrent requests with the same build ID
	buildID := "singleflight-test-id"
	ctx := context.Background()

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
	}, newMetrics(nil))
	require.NoError(t, err)

	ctx := context.Background()
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
