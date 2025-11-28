package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

type DefaultHTTPClient struct {
	client *http.Client
}

func NewHTTPClient() HTTPClient {
	return &DefaultHTTPClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *DefaultHTTPClient) Get(url string) (*http.Response, error) {
	return c.client.Get(url)
}

// MavenService handles downloading JAR files from Maven Central.
type MavenService struct {
	client HTTPClient
}

func NewMavenService(client HTTPClient) *MavenService {
	if client == nil {
		client = NewHTTPClient()
	}
	return &MavenService{client: client}
}

func (s *MavenService) FetchJAR(groupId, artifactId, version string) ([]byte, error) {
	groupIdPath := strings.ReplaceAll(groupId, ".", "/")
	urlStr := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.jar",
		groupIdPath, artifactId, version, artifactId, version)

	resp, err := s.client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	return data, nil
}

// DepsDevService handles querying the deps.dev API for GitHub repository information.
type DepsDevService struct {
	client HTTPClient
}

func NewDepsDevService(client HTTPClient) *DepsDevService {
	if client == nil {
		client = NewHTTPClient()
	}
	return &DepsDevService{client: client}
}

// DepsDevVersionResponse represents the response from deps.dev API.
type DepsDevVersionResponse struct {
	Links []struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"links"`
}

// ResolveRepo queries deps.dev API to find the GitHub repository for a Maven artifact.
func (s *DepsDevService) ResolveRepo(groupId, artifactId, version string) (owner, repo string, err error) {
	packageKey := fmt.Sprintf("%s:%s", groupId, artifactId)
	apiURL := fmt.Sprintf("https://api.deps.dev/v3/systems/maven/packages/%s/versions/%s",
		url.PathEscape(packageKey), url.PathEscape(version))

	resp, err := s.client.Get(apiURL)
	if err != nil {
		return "", "", fmt.Errorf("deps.dev API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("deps.dev API returned HTTP %d", resp.StatusCode)
	}

	var depsDevResp DepsDevVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&depsDevResp); err != nil {
		return "", "", fmt.Errorf("failed to parse deps.dev response: %w", err)
	}

	// Look for SOURCE_REPO link with GitHub URL
	for _, link := range depsDevResp.Links {
		if strings.Contains(link.URL, "github.com") {
			owner, repo, err := ExtractGitHubRepoFromURL(link.URL)
			if err == nil {
				return owner, repo, nil
			}
		}
	}

	return "", "", fmt.Errorf("no GitHub repository found in deps.dev response")
}

// GitHubTagService handles querying GitHub API for tags.
type GitHubTagService struct {
	client    HTTPClient
	authToken string
}

func NewGitHubTagService(client HTTPClient) *GitHubTagService {
	if client == nil {
		client = NewHTTPClient()
	}
	authToken := os.Getenv("GITHUB_TOKEN")
	return &GitHubTagService{
		client:    client,
		authToken: authToken,
	}
}

type GitHubTag struct {
	Name string `json:"name"`
}

func (s *GitHubTagService) FindTagForVersion(owner, repo, version string) (string, error) {
	versionNormalized := strings.TrimPrefix(version, "v")
	hasVPrefix := strings.HasPrefix(version, "v")

	// Build candidates list prioritizing exact match
	var candidates []string
	if hasVPrefix {
		candidates = []string{version, versionNormalized}
	} else {
		candidates = []string{version, "v" + versionNormalized}
	}

	makeRequest := func(urlStr string) (*http.Response, error) {
		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			return nil, err
		}
		if s.authToken != "" {
			req.Header.Set("Authorization", fmt.Sprintf("token %s", s.authToken))
		}
		req.Header.Set("User-Agent", "pyroscope-jar-mapper")
		if defaultClient, ok := s.client.(*DefaultHTTPClient); ok {
			return defaultClient.client.Do(req)
		}
		return s.client.Get(urlStr)
	}

	// Try direct lookup for each candidate
	for _, candidate := range candidates {
		refURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/refs/tags/%s", owner, repo, candidate)
		resp, err := makeRequest(refURL)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var refData struct {
					Ref string `json:"ref"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&refData); err == nil {
					if strings.HasPrefix(refData.Ref, "refs/tags/") {
						return strings.TrimPrefix(refData.Ref, "refs/tags/"), nil
					}
				}
			} else if resp.StatusCode == http.StatusForbidden {
				return "", fmt.Errorf("GitHub API rate limited. Set GITHUB_TOKEN environment variable")
			}
		}
	}

	// Fallback: fetch tags list with pagination
	nextURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=100", owner, repo)

	for nextURL != "" {
		resp, err := makeRequest(nextURL)
		if err != nil {
			return "", fmt.Errorf("failed to query GitHub API: %w", err)
		}

		if resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return "", fmt.Errorf("GitHub API rate limited. Set GITHUB_TOKEN environment variable")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
		}

		var tags []GitHubTag
		if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("failed to parse GitHub API response: %w", err)
		}

		// Try exact matches
		for _, candidate := range candidates {
			for _, tag := range tags {
				if tag.Name == candidate {
					resp.Body.Close()
					return tag.Name, nil
				}
			}
		}

		// Try normalized matching
		for _, tag := range tags {
			tagNormalized := strings.TrimPrefix(tag.Name, "v")
			if tagNormalized == versionNormalized {
				resp.Body.Close()
				return tag.Name, nil
			}
		}

		nextURL = parseNextPageURL(resp.Header.Get("Link"))
		resp.Body.Close()
	}

	// Fallback: return version with "v" prefix
	return "v" + versionNormalized, nil
}

// parseNextPageURL extracts the "next" URL from a GitHub Link header.
// Example: <https://api.github.com/repos/o/r/tags?page=2>; rel="next", <...>; rel="last"
func parseNextPageURL(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	// Split by comma to get individual links
	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) < 2 {
			continue
		}

		// Check if this is the "next" link
		relPart := strings.TrimSpace(parts[1])
		if relPart == `rel="next"` {
			// Extract URL from angle brackets
			urlPart := strings.TrimSpace(parts[0])
			if strings.HasPrefix(urlPart, "<") && strings.HasSuffix(urlPart, ">") {
				return urlPart[1 : len(urlPart)-1]
			}
		}
	}

	return ""
}
