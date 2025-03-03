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
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
)

type DebuginfodClient interface {
	FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error)
}

type debuginfodClientConfig struct {
	baseURL     string
	maxRetries  int
	backoffTime time.Duration
	// userAgent   string
	httpClient *http.Client
}
type debuginfodClient struct {
	cfg     debuginfodClientConfig
	metrics *Metrics

	// In-memory cache of build IDs to file paths
	cache *ristretto.Cache

	// Track in-flight requests to prevent duplicate fetches
	inFlightRequests      map[string]*inFlightRequest
	inFlightRequestsMutex sync.Mutex
}

// inFlightRequest represents an ongoing fetch operation
type inFlightRequest struct {
	done chan struct{}
	data []byte
	err  error
}

func NewDebuginfodClient(baseURL string, metrics *Metrics) DebuginfodClient {
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
		cache = nil
	}

	return &debuginfodClient{
		cfg: debuginfodClientConfig{
			baseURL:     baseURL,
			maxRetries:  3,
			backoffTime: time.Second,
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
		},
		metrics:          metrics,
		cache:            cache,
		inFlightRequests: make(map[string]*inFlightRequest),
	}
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
func (c *debuginfodClient) FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	start := time.Now()
	var lastErr error
	c.metrics.debuginfodRequestsTotal.Inc()
	defer func() {
		status := "success"
		if lastErr != nil {
			status = "error"
			c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("http").Inc()
		}
		c.metrics.debuginfodRequestDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	sanitizedBuildID, err := sanitizeBuildID(buildID)
	if err != nil {
		c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("invalid_id").Inc()
		return nil, err
	}

	// Check in-memory cache first
	if c.cache != nil {
		if data, found := c.cache.Get(sanitizedBuildID); found {
			return io.NopCloser(bytes.NewReader(data.([]byte))), nil
		}
	}

	// Check if there's already a request in flight for this build ID
	c.inFlightRequestsMutex.Lock()
	req, inFlight := c.inFlightRequests[sanitizedBuildID]
	if inFlight {
		// There's already a request in flight, wait for it to complete
		done := req.done
		c.inFlightRequestsMutex.Unlock()

		select {
		case <-done:
			if req.err != nil {
				lastErr = req.err
				return nil, req.err
			}
			return io.NopCloser(bytes.NewReader(req.data)), nil
		case <-ctx.Done():
			lastErr = ctx.Err()
			return nil, ctx.Err()
		}
	}

	// Create a new in-flight request
	req = &inFlightRequest{
		done: make(chan struct{}),
	}
	c.inFlightRequests[sanitizedBuildID] = req
	c.inFlightRequestsMutex.Unlock()

	// Ensure we clean up the in-flight request when we're done
	defer func() {
		c.inFlightRequestsMutex.Lock()
		delete(c.inFlightRequests, sanitizedBuildID)
		close(req.done)
		c.inFlightRequestsMutex.Unlock()
	}()

	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.cfg.baseURL, sanitizedBuildID)
	var data []byte

	// Implement retries with exponential backoff
	for attempt := 0; attempt < c.cfg.maxRetries; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			time.Sleep(c.cfg.backoffTime * time.Duration(attempt))
		}

		data, err = c.doRequest(ctx, url)
		if err == nil {
			break
		}

		lastErr = err
		if !isRetryableError(err) {
			break
		}
	}

	if lastErr != nil {
		req.err = fmt.Errorf("failed to fetch debuginfo after %d attempts: %w", c.cfg.maxRetries, lastErr)
		return nil, req.err
	}

	// Store in cache
	if c.cache != nil {
		// The cost is the size of the data in bytes
		c.cache.Set(sanitizedBuildID, data, int64(len(data)))
	}

	req.data = data
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (c *debuginfodClient) doRequest(ctx context.Context, url string) ([]byte, error) {
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
		return nil, fmt.Errorf("unexpected HTTP status: %d %s", resp.StatusCode, resp.Status)
	}

	// Read the entire response body into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.ContentLength > 0 {
		c.metrics.debuginfodFileSize.Observe(float64(resp.ContentLength))
	} else {
		c.metrics.debuginfodFileSize.Observe(float64(len(data)))
	}

	return data, nil
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	if os.IsTimeout(err) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Temporary()
	}

	// All 5xx errors are retryable
	if strings.Contains(err.Error(), "status: 5") {
		return true
	}

	return false
}

// sanitizeBuildID ensures that the buildID is a safe and valid string for use in file paths.
func sanitizeBuildID(buildID string) (string, error) {
	// Allow only alphanumeric characters, dashes, and underscores.
	validBuildID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validBuildID.MatchString(buildID) {
		return "", fmt.Errorf("invalid build ID: %s", buildID)
	}
	return buildID, nil
}
