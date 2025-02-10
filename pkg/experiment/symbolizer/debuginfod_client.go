package symbolizer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type DebuginfodClient interface {
	FetchDebuginfo(buildID string) (string, error)
}

type debuginfodClient struct {
	baseURL    string
	metrics    *Metrics
	httpClient *http.Client
}

func NewDebuginfodClient(baseURL string, metrics *Metrics) DebuginfodClient {
	// Create a default transport with reasonable timeouts
	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Overall timeout for requests
	}

	return &debuginfodClient{
		baseURL:    baseURL,
		metrics:    metrics,
		httpClient: client,
	}
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
func (c *debuginfodClient) FetchDebuginfo(buildID string) (string, error) {
	start := time.Now()
	var err error
	c.metrics.debuginfodRequestsTotal.Inc()
	defer func() {
		status := "success"
		if err != nil {
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

	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.baseURL, sanitizedBuildID)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("request_creation").Inc()
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := c.httpClient.Do(req)
	//resp, err := http.Get(url)
	if err != nil {
		c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("http").Inc()
		c.metrics.debuginfodRequestDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
		return "", fmt.Errorf("failed to fetch debuginfod: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("http").Inc()
		c.metrics.debuginfodRequestDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
		return "", fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	// Record file size from Content-Length if available
	if contentLength := resp.ContentLength; contentLength > 0 {
		c.metrics.debuginfodFileSize.Observe(float64(contentLength))
	}

	// TODO: Avoid file operations and handle debuginfo in memory.
	// Save the debuginfo to a temporary file
	tempDir := os.TempDir()
	filePath := filepath.Join(tempDir, fmt.Sprintf("%s.elf", sanitizedBuildID))
	outFile, err := os.Create(filePath)
	if err != nil {
		c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("file_create").Inc()
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		c.metrics.debuginfodRequestErrorsTotal.WithLabelValues("write").Inc()
		return "", fmt.Errorf("failed to write debuginfod to file: %w", err)
	}

	return filePath, nil
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
