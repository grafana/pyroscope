package symbolizer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DebuginfodClient interface {
	FetchDebuginfo(ctx context.Context, buildID string) (string, error)
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
}

func NewDebuginfodClient(baseURL string, metrics *Metrics) DebuginfodClient {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
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
		metrics: metrics,
	}
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
func (c *debuginfodClient) FetchDebuginfo(ctx context.Context, buildID string) (string, error) {
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
		return "", err
	}

	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.cfg.baseURL, sanitizedBuildID)

	// Implement retries with exponential backoff
	for attempt := 0; attempt < c.cfg.maxRetries; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			time.Sleep(c.cfg.backoffTime * time.Duration(attempt))
		}

		filePath, err := c.doRequest(ctx, url, sanitizedBuildID)
		if err == nil {
			return filePath, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break
		}
	}

	return "", fmt.Errorf("failed to fetch debuginfo after %d attempts: %w", c.cfg.maxRetries, lastErr)
}

func (c *debuginfodClient) doRequest(ctx context.Context, url, buildID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := c.cfg.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %d %s", resp.StatusCode, resp.Status)
	}

	return c.saveDebugInfo(resp.Body, buildID, resp.ContentLength)
}

func (c *debuginfodClient) saveDebugInfo(body io.Reader, buildID string, contentLength int64) (string, error) {
	if contentLength > 0 {
		c.metrics.debuginfodFileSize.Observe(float64(contentLength))
	}

	tempDir := os.TempDir()
	filePath := filepath.Join(tempDir, fmt.Sprintf("%s.elf", buildID))

	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, body); err != nil {
		os.Remove(filePath) // Clean up on error
		return "", fmt.Errorf("failed to write debug info: %w", err)
	}

	return filePath, nil
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
