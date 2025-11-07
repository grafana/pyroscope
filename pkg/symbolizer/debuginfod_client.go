package symbolizer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/backoff"
	"golang.org/x/sync/singleflight"

	"github.com/dgraph-io/ristretto/v2"
)

// DebuginfodClientConfig holds configuration for the debuginfod client.
type DebuginfodClientConfig struct {
	BaseURL       string
	HTTPClient    *http.Client
	BackoffConfig backoff.Config
	UserAgent     string

	NotFoundCacheMaxItems int64
	NotFoundCacheTTL      time.Duration
}

// DebuginfodHTTPClient implements the DebuginfodClient interface using HTTP.
type DebuginfodHTTPClient struct {
	cfg     DebuginfodClientConfig
	metrics *metrics
	logger  log.Logger

	// Used to deduplicate concurrent requests for the same build ID
	group singleflight.Group

	notFoundCache *ristretto.Cache[string, bool]
}

// NewDebuginfodClient creates a new client for fetching debug information from a debuginfod server.
func NewDebuginfodClient(logger log.Logger, baseURL string, metrics *metrics) (*DebuginfodHTTPClient, error) {
	return NewDebuginfodClientWithConfig(logger, DebuginfodClientConfig{
		BaseURL: baseURL,
		//UserAgent:  "Pyroscope-Symbolizer/1.0",
		BackoffConfig: backoff.Config{
			MinBackoff: 1 * time.Second,
			MaxBackoff: 10 * time.Second,
			MaxRetries: 3,
		},
		NotFoundCacheMaxItems: 100000,
		NotFoundCacheTTL:      7 * 24 * time.Hour,
	}, metrics)
}

// NewDebuginfodClientWithConfig creates a new client with the specified configuration.
func NewDebuginfodClientWithConfig(logger log.Logger, cfg DebuginfodClientConfig, metrics *metrics) (*DebuginfodHTTPClient, error) {
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

	cache, err := ristretto.NewCache(&ristretto.Config[string, bool]{
		NumCounters: cfg.NotFoundCacheMaxItems * 10,
		MaxCost:     cfg.NotFoundCacheMaxItems,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create not-found cache: %w", err)
	}

	client := &DebuginfodHTTPClient{
		cfg: DebuginfodClientConfig{
			BaseURL:       cfg.BaseURL,
			UserAgent:     cfg.UserAgent,
			HTTPClient:    httpClient,
			BackoffConfig: cfg.BackoffConfig,
		},
		metrics:       metrics,
		logger:        logger,
		notFoundCache: cache,
	}

	return client, nil
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
func (c *DebuginfodHTTPClient) FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	start := time.Now()
	status := statusSuccess
	defer func() {
		c.metrics.debuginfodRequestDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	sanitizedBuildID, err := sanitizeBuildID(buildID)
	if err != nil {
		status = statusErrorInvalidID
		return nil, err
	}

	if found, _ := c.notFoundCache.Get(sanitizedBuildID); found {
		status = statusErrorNotFound
		c.metrics.cacheOperations.WithLabelValues("not_found", "get", statusSuccess).Inc()
		return nil, buildIDNotFoundError{buildID: sanitizedBuildID}
	}
	c.metrics.cacheOperations.WithLabelValues("not_found", "get", "miss").Inc()

	v, err, _ := c.group.Do(sanitizedBuildID, func() (interface{}, error) {
		return c.fetchDebugInfoWithRetries(ctx, sanitizedBuildID)
	})

	if err != nil {
		var bnfErr buildIDNotFoundError
		switch {
		case errors.As(err, &bnfErr):
			status = statusErrorNotFound
		case errors.Is(err, context.Canceled):
			status = statusErrorCanceled
		case errors.Is(err, context.DeadlineExceeded):
			status = statusErrorTimeout
		case isInvalidBuildIDError(err):
			status = statusErrorInvalidID
		default:
			if statusCode, ok := isHTTPStatusError(err); ok {
				status = categorizeHTTPStatusCode(statusCode)
			} else {
				status = statusErrorOther
			}
		}
		return nil, err
	}

	data := v.([]byte)
	return io.NopCloser(bytes.NewReader(data)), nil
}

// doRequest performs an HTTP request to the specified URL and returns the response body.
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

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errorBody := string(data)

		// Truncate large error responses
		if len(errorBody) > 1000 {
			errorBody = errorBody[:1000] + "... [truncated]"
		}
		return nil, httpStatusError{
			statusCode: resp.StatusCode,
			body:       errorBody,
		}
	}

	c.metrics.debuginfodFileSize.Observe(float64(len(data)))

	return data, nil
}

// fetchDebugInfoWithRetries attempts to fetch debug info with retries on transient errors.
func (c *DebuginfodHTTPClient) fetchDebugInfoWithRetries(ctx context.Context, sanitizedBuildID string) ([]byte, error) {
	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.cfg.BaseURL, sanitizedBuildID)
	var data []byte

	// Use dskit backoff for retries with exponential backoff
	backOff := backoff.New(ctx, c.cfg.BackoffConfig)

	attempt := func() ([]byte, error) {
		return c.doRequest(ctx, url)
	}

	var lastErr error
	for backOff.Ongoing() {
		data, err := attempt()
		if err == nil {
			return data, nil
		}

		// Don't retry on 404 errors
		if statusCode, isHTTPErr := isHTTPStatusError(err); isHTTPErr && statusCode == http.StatusNotFound {
			c.notFoundCache.SetWithTTL(sanitizedBuildID, true, 1, c.cfg.NotFoundCacheTTL)
			c.notFoundCache.Wait()
			c.metrics.cacheOperations.WithLabelValues("not_found", "set", statusSuccess).Inc()
			c.metrics.cacheSizeBytes.WithLabelValues("not_found").Set(float64(c.notFoundCache.Metrics.CostAdded()))
			return nil, buildIDNotFoundError{buildID: sanitizedBuildID}
		}

		lastErr = err

		if !isRetryableError(err) {
			break
		}

		backOff.Wait()
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch debuginfo after %d attempts: %w", backOff.NumRetries(), lastErr)
	}

	return data, nil
}

// categorizeHTTPStatusCode maps HTTP status codes to metric status strings.
func categorizeHTTPStatusCode(statusCode int) string {
	switch {
	case statusCode == http.StatusNotFound:
		return statusErrorNotFound
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return statusErrorUnauthorized
	case statusCode == http.StatusTooManyRequests:
		return statusErrorRateLimited
	case statusCode >= 400 && statusCode < 500:
		return statusErrorClientError
	case statusCode >= 500:
		return statusErrorServerError
	default:
		return statusErrorHTTPOther
	}
}

// isRetryableError determines if an error should trigger a retry attempt.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	if isInvalidBuildIDError(err) {
		return false
	}

	var bnfErr buildIDNotFoundError
	if errors.As(err, &bnfErr) {
		return false
	}

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

	if os.IsTimeout(err) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

// sanitizeBuildID ensures that the buildID is a safe and valid string for use in file paths.
// It validates that the build ID contains only alphanumeric characters, underscores, and hyphens.
// This prevents potential security issues like path traversal attacks.
func sanitizeBuildID(buildID string) (string, error) {
	if buildID == "" {
		return "", nil
	}

	validBuildID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validBuildID.MatchString(buildID) {
		return "", invalidBuildIDError{buildID: buildID}
	}
	return buildID, nil
}
