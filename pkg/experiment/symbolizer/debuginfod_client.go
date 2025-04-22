// Package symbolizer provides functionality for symbolizing profiles.
package symbolizer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"golang.org/x/sync/singleflight"
)

// DebuginfodClientConfig holds configuration for the debuginfod client.
// This allows for more flexible configuration when creating a client.
type DebuginfodClientConfig struct {
	// BaseURL is the URL of the debuginfod server
	BaseURL string

	// MaxRetries is the maximum number of retry attempts for failed requests
	MaxRetries int

	// HTTPClient is the HTTP client to use for requests
	// If nil, a default client will be created
	HTTPClient *http.Client

	// BackoffConfig configures the retry backoff behavior
	BackoffConfig backoff.Config

	// UserAgent is the User-Agent header to use for requests
	UserAgent string

	// CacheConfig configures the in-memory cache
	CacheConfig struct {
		// MaxSizeBytes is the maximum size of the cache in bytes
		// Default is 2GB
		MaxSizeBytes int64

		// NumCounters is the number of keys to track frequency of
		// Default is 10M
		NumCounters int64
	}
}

// DebuginfodHTTPClient implements the DebuginfodClient interface using HTTP.
// It fetches debug information from a debuginfod server and caches the results.
type DebuginfodHTTPClient struct {
	cfg     DebuginfodClientConfig
	metrics *Metrics
	logger  log.Logger

	// Used to deduplicate concurrent requests for the same build ID
	group singleflight.Group
}

// NewDebuginfodClient creates a new client for fetching debug information from a debuginfod server.
// It sets up an HTTP client, in-memory cache, and configures metrics.
func NewDebuginfodClient(logger log.Logger, baseURL string, metrics *Metrics) (*DebuginfodHTTPClient, error) {
	return NewDebuginfodClientWithConfig(logger, DebuginfodClientConfig{
		BaseURL:    baseURL,
		MaxRetries: 3,
		UserAgent:  "Pyroscope-Symbolizer/1.0",
		BackoffConfig: backoff.Config{
			MinBackoff: 1 * time.Second,
			MaxBackoff: 10 * time.Second,
			MaxRetries: 3,
		},
	}, metrics)
}

// NewDebuginfodClientWithConfig creates a new client with the specified configuration.
// This allows for more flexible configuration when creating a client.
func NewDebuginfodClientWithConfig(logger log.Logger, cfg DebuginfodClientConfig, metrics *Metrics) (*DebuginfodHTTPClient, error) {
	// Create HTTP client if not provided
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport := &http.Transport{
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		}

		httpClient = &http.Client{
			Transport: transport,
			Timeout:   120 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("stopped after 3 redirects")
				}
				return nil
			},
		}
	}

	return &DebuginfodHTTPClient{
		cfg: DebuginfodClientConfig{
			BaseURL:       cfg.BaseURL,
			MaxRetries:    cfg.MaxRetries,
			UserAgent:     cfg.UserAgent,
			HTTPClient:    httpClient,
			BackoffConfig: cfg.BackoffConfig,
			CacheConfig:   cfg.CacheConfig,
		},
		metrics: metrics,
		logger:  logger,
	}, nil
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
// It returns an io.ReadCloser that the caller must close when done.
// The method handles caching, retries, and error categorization.
func (c *DebuginfodHTTPClient) FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	start := time.Now()
	status := StatusSuccess
	defer func() {
		c.metrics.debuginfodRequestDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	level.Debug(c.logger).Log(
		"msg", "symbolizer: starting debuginfod fetch",
		"build_id", buildID,
	)

	sanitizedBuildID, err := sanitizeBuildID(buildID)
	if err != nil {
		status = StatusErrorInvalidID
		return nil, err
	}

	// Check if there's already a request in flight for this build ID
	level.Debug(c.logger).Log(
		"msg", "symbolizer: making debuginfod request",
		"build_id", sanitizedBuildID,
	)
	v, err, _ := c.group.Do(sanitizedBuildID, func() (interface{}, error) {
		return c.fetchDebugInfoWithRetries(ctx, sanitizedBuildID)
	})

	if err != nil {
		// Categorize errors based on type for better metrics
		if errors.Is(err, context.Canceled) {
			status = StatusErrorCanceled
		} else if errors.Is(err, context.DeadlineExceeded) {
			status = StatusErrorTimeout
		} else if isInvalidBuildIDError(err) {
			status = StatusErrorInvalidID
		} else if statusCode, ok := isHTTPStatusError(err); ok {
			status = categorizeHTTPStatusCode(statusCode)
		} else {
			status = StatusErrorOther
		}
		return nil, err
	}

	data := v.([]byte)
	c.metrics.debuginfodFileSize.Observe(float64(len(data)))
	level.Debug(c.logger).Log(
		"msg", "symbolizer: debuginfod fetch successful",
		"build_id", sanitizedBuildID,
		"size", len(data),
	)
	return io.NopCloser(bytes.NewReader(data)), nil
}

// doRequest performs an HTTP request to the specified URL and returns the response body.
// It handles HTTP errors and response body reading.
func (c *DebuginfodHTTPClient) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept-Encoding", "gzip, deflate")
	if c.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", c.cfg.UserAgent)
	}

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, httpStatusError{
			statusCode: resp.StatusCode,
			status:     resp.Status,
		}
	}

	// Read the entire response body into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.metrics.debuginfodFileSize.Observe(float64(len(data)))

	return data, nil
}

// fetchDebugInfoWithRetries attempts to fetch debug info with retries on transient errors.
// It uses exponential backoff for retries and handles various error conditions.
func (c *DebuginfodHTTPClient) fetchDebugInfoWithRetries(ctx context.Context, sanitizedBuildID string) ([]byte, error) {
	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.cfg.BaseURL, sanitizedBuildID)
	var data []byte
	var lastErr error

	// Use dskit backoff for retries with exponential backoff
	backOff := backoff.New(ctx, c.cfg.BackoffConfig)

	// Define the attempt function that will be retried
	attempt := func() bool {
		var err error
		data, err = c.doRequest(ctx, url)
		if err == nil {
			return true // Success, no need to retry
		}

		// Special handling for 404 Not Found - convert to a specific error type
		statusCode, isHTTPErr := isHTTPStatusError(err)
		if isHTTPErr && statusCode == http.StatusNotFound {
			lastErr = buildIDNotFoundError{buildID: sanitizedBuildID}
			return true
		}

		// Store the error for later reporting
		lastErr = err

		// Determine if we should retry based on the error type
		return isRetryableError(err)
	}

	// Retry loop with backoff
	for backOff.Ongoing() {
		if attempt() {
			break
		}
		// Log retry attempt
		if c.logger != nil {
			level.Debug(c.logger).Log(
				"msg", "Retrying debuginfod request",
				"url", url,
				"attempt", backOff.NumRetries(),
				"error", lastErr,
			)
		}
		backOff.Wait()
	}

	// If we still have an error after all retries, return it with context
	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch debuginfo after %d attempts: %w", backOff.NumRetries(), lastErr)
	}

	return data, nil
}

// categorizeHTTPStatusCode maps HTTP status codes to metric status strings.
// This helps in categorizing errors for better metrics and monitoring.
func categorizeHTTPStatusCode(statusCode int) string {
	switch {
	case statusCode == http.StatusNotFound:
		return StatusErrorNotFound
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return StatusErrorUnauthorized
	case statusCode == http.StatusTooManyRequests:
		return StatusErrorRateLimited
	case statusCode >= 400 && statusCode < 500:
		return StatusErrorClientError
	case statusCode >= 500:
		return StatusErrorServerError
	default:
		return StatusErrorHTTPOther
	}
}

// isRetryableError determines if an error should trigger a retry attempt.
// Some errors (like context cancellation or 404s) should not be retried,
// while others (like network timeouts or 5xx errors) should be.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on context cancellation or deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	// Don't retry on invalid build ID
	if isInvalidBuildIDError(err) {
		return false
	}

	// Don't retry on not found errors
	if _, ok := err.(buildIDNotFoundError); ok {
		return false
	}

	// Check HTTP status errors
	if statusCode, ok := isHTTPStatusError(err); ok {
		// Don't retry 4xx client errors except for 429 (too many requests)
		if statusCode == http.StatusTooManyRequests {
			return true
		}
		if statusCode >= 400 && statusCode < 500 {
			return false
		}
		// Retry on 5xx server errors
		return statusCode >= 500
	}

	// Retry on network timeouts
	if os.IsTimeout(err) {
		return true
	}

	// Check for URL errors that are temporary
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Temporary()
	}

	return false
}

// sanitizeBuildID ensures that the buildID is a safe and valid string for use in file paths.
// It validates that the build ID contains only alphanumeric characters, underscores, and hyphens.
// This prevents potential security issues like path traversal attacks.
func sanitizeBuildID(buildID string) (string, error) {
	if buildID == "" {
		return "", invalidBuildIDError{buildID: buildID}
	}

	// Only allow alphanumeric characters, underscores, and hyphens
	validBuildID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validBuildID.MatchString(buildID) {
		return "", invalidBuildIDError{buildID: buildID}
	}
	return buildID, nil
}
