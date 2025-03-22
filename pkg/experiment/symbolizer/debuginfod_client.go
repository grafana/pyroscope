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

	"github.com/dgraph-io/ristretto"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/backoff"
	"golang.org/x/sync/singleflight"
)

type debuginfodClientConfig struct {
	baseURL    string
	maxRetries int
	httpClient *http.Client
	backoffCfg backoff.Config
}
type DebuginfodHTTPClient struct {
	cfg     debuginfodClientConfig
	metrics *Metrics

	// In-memory cache of build IDs to file paths
	cache *ristretto.Cache

	group singleflight.Group
	l     log.Logger
}

func NewDebuginfodClient(l log.Logger, baseURL string, metrics *Metrics) (*DebuginfodHTTPClient, error) {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M)
		MaxCost:     2 << 30, // maximum cost of cache (2GB)
		BufferItems: 64,      // number of keys per Get buffer
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create debuginfod cache: %w", err)
	}

	return &DebuginfodHTTPClient{
		cfg: debuginfodClientConfig{
			baseURL:    baseURL,
			maxRetries: 3,
			// userAgent:   "Pyroscope-Symbolizer/1.0",
			httpClient: &http.Client{
				Transport: transport,
				Timeout:   30 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					if len(via) >= 3 {
						return fmt.Errorf("stopped after 3 redirects")
					}
					return nil
				},
			},
			backoffCfg: backoff.Config{
				MinBackoff: 1 * time.Second,
				MaxBackoff: 10 * time.Second,
				MaxRetries: 3,
			},
		},
		metrics: metrics,
		cache:   cache,
		l:       l,
	}, nil
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
func (c *DebuginfodHTTPClient) FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	start := time.Now()
	status := StatusSuccess
	defer func() {
		c.metrics.debuginfodRequestDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	sanitizedBuildID, err := sanitizeBuildID(buildID)
	if err != nil {
		status = StatusErrorInvalidID
		return nil, err
	}

	// Check in-memory cache first
	if c.cache != nil {
		if data, found := c.cache.Get(sanitizedBuildID); found {
			status = StatusCacheHit
			return io.NopCloser(bytes.NewReader(data.([]byte))), nil
		}
	}

	// Check if there's already a request in flight for this build ID
	v, err, _ := c.group.Do(sanitizedBuildID, func() (interface{}, error) {
		return c.fetchDebugInfoWithRetries(ctx, sanitizedBuildID)
	})

	if err != nil {
		// Categorize errors based on type
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
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (c *DebuginfodHTTPClient) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := c.cfg.httpClient.Do(req)
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

func (c *DebuginfodHTTPClient) fetchDebugInfoWithRetries(ctx context.Context, buildID string) ([]byte, error) {
	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.cfg.baseURL, buildID)
	var data []byte
	var lastErr error

	// Use dskit backoff for retries
	backOff := backoff.New(ctx, c.cfg.backoffCfg)

	attempt := func() bool {
		var err error
		data, err = c.doRequest(ctx, url)
		if err == nil {
			return true
		}

		statusCode, isHTTPErr := isHTTPStatusError(err)
		if isHTTPErr && statusCode == http.StatusNotFound {
			lastErr = buildIDNotFoundError{buildID: buildID}
			return true
		}

		lastErr = err
		return isRetryableError(err)
	}

	for backOff.Ongoing() {
		if attempt() {
			break
		}
		backOff.Wait()
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch debuginfo after %d attempts: %w", backOff.NumRetries(), lastErr)
	}

	// Store in cache
	if c.cache != nil {
		// The cost is the size of the data in bytes
		c.cache.Set(buildID, data, int64(len(data)))
	}

	return data, nil
}

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

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Temporary()
	}

	return false
}

// sanitizeBuildID ensures that the buildID is a safe and valid string for use in file paths.
func sanitizeBuildID(buildID string) (string, error) {
	validBuildID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validBuildID.MatchString(buildID) {
		return "", fmt.Errorf("invalid build ID: %s", buildID)
	}
	return buildID, nil
}
