package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// This tool updates the SHA256 checksums in pkg/embedded/grafana/grafana.go
// by downloading the release artifacts and calculating their actual checksums.
//
// Usage:
//   go run tools/update-embedded-checksums.go

const grafanaGoPath = "pkg/embedded/grafana/grafana.go"

type artifact struct {
	url      string
	checksum string
	lineNum  int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("Updating embedded Grafana checksums...")
	fmt.Println()

	// Read the file
	content, err := os.ReadFile(grafanaGoPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", grafanaGoPath, err)
	}

	// Extract artifacts (URL + checksum pairs)
	artifacts, err := extractArtifacts(string(content))
	if err != nil {
		return fmt.Errorf("failed to extract artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		fmt.Println("No artifacts found to update")
		return nil
	}

	// Update checksums
	updatedContent := string(content)
	updatedCount := 0

	for _, artifact := range artifacts {
		fmt.Printf("Processing: %s\n", extractFilename(artifact.url))
		fmt.Printf("  Current checksum: %s\n", artifact.checksum)

		actualChecksum, err := downloadAndChecksum(artifact.url)
		if err != nil {
			return fmt.Errorf("failed to process %s: %w", artifact.url, err)
		}

		fmt.Printf("  Actual checksum:  %s\n", actualChecksum)

		if artifact.checksum != actualChecksum {
			fmt.Println("  ✓ Updating checksum")
			updatedContent = strings.ReplaceAll(
				updatedContent,
				fmt.Sprintf(`mustHexDecode("%s")`, artifact.checksum),
				fmt.Sprintf(`mustHexDecode("%s")`, actualChecksum),
			)
			updatedCount++
		} else {
			fmt.Println("  ✓ Checksum matches")
		}
		fmt.Println()
	}

	// Write back if changes were made
	if updatedCount > 0 {
		if err := os.WriteFile(grafanaGoPath, []byte(updatedContent), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", grafanaGoPath, err)
		}
		fmt.Printf("✓ Updated %d checksum(s) in %s\n", updatedCount, grafanaGoPath)
	} else {
		fmt.Println("✓ All checksums are already correct")
	}

	return nil
}

func extractArtifacts(content string) ([]artifact, error) {
	var artifacts []artifact

	// Pattern to match URL and subsequent Sha256Sum
	// We need to find URL: "...", then find the next Sha256Sum: mustHexDecode("...")
	lines := strings.Split(content, "\n")

	var currentURL string
	for i, line := range lines {
		// Look for URL line
		urlPattern, err := regexp.Compile(`URL:\s+"([^"]+)"`)
		if err != nil {
			return nil, err
		}

		if matches := urlPattern.FindStringSubmatch(line); len(matches) > 1 {
			currentURL = matches[1]
			continue
		}

		// Look for Sha256Sum line if we have a current URL
		if currentURL != "" {
			checksumPattern := regexp.MustCompile(`Sha256Sum:\s+mustHexDecode\("([a-f0-9]+)"\)`)
			if matches := checksumPattern.FindStringSubmatch(line); len(matches) > 1 {
				artifacts = append(artifacts, artifact{
					url:      currentURL,
					checksum: matches[1],
					lineNum:  i + 1,
				})
				currentURL = "" // Reset for next artifact
			}
		}
	}

	return artifacts, nil
}

func downloadAndChecksum(url string) (string, error) {
	fmt.Printf("  Downloading...\n")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "pyroscope/update-embedded-checksums")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func extractFilename(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return url
}
