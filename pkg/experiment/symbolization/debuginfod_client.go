package symbolization

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type DebuginfodClient interface {
	FetchDebuginfo(buildID string) (string, error)
}

type debuginfodClient struct {
	baseURL string
}

func NewDebuginfodClient(baseURL string) DebuginfodClient {
	return &debuginfodClient{
		baseURL: baseURL,
	}
}

// FetchDebuginfo fetches the debuginfo file for a specific build ID.
func (c *debuginfodClient) FetchDebuginfo(buildID string) (string, error) {
	url := fmt.Sprintf("%s/buildid/%s/debuginfo", c.baseURL, buildID)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch debuginfod: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	// Save the debuginfo to a temporary file
	tempDir := os.TempDir()
	filePath := filepath.Join(tempDir, fmt.Sprintf("%s.elf", buildID))
	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write debuginfod to file: %w", err)
	}

	return filePath, nil
}
