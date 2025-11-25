package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

type DefaultHTTPClient struct {
	client *http.Client
}

// NewHTTPClient creates a new DefaultHTTPClient with a 5-second timeout.
func NewHTTPClient() HTTPClient {
	return &DefaultHTTPClient{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *DefaultHTTPClient) Get(url string) (*http.Response, error) {
	return c.client.Get(url)
}

// RepoResolver is an interface for resolving GitHub repositories from Maven coordinates.
type RepoResolver interface {
	ResolveRepo(groupId, artifactId, version string) (owner, repo string, err error)
}

// MavenService handles fetching POM files from Maven Central.
type MavenService struct {
	client    HTTPClient
	pomParser *POMParser
}

func NewMavenService(client HTTPClient, pomParser *POMParser) *MavenService {
	if client == nil {
		client = NewHTTPClient()
	}
	return &MavenService{
		client:    client,
		pomParser: pomParser,
	}
}

// MavenSearchResponse represents the response from Maven Central search API.
type MavenSearchResponse struct {
	Response struct {
		Docs []struct {
			GroupID    string `json:"g"`
			ArtifactID string `json:"a"`
			Version    string `json:"latestVersion"`
		} `json:"docs"`
	} `json:"response"`
}

// FetchPOM fetches a POM file from Maven Central by trying common groupId patterns.
func (s *MavenService) FetchPOM(artifactId, version string) ([]byte, string, error) {
	// Try common groupId patterns
	groupIds := []string{
		strings.ToLower(artifactId),
		strings.ReplaceAll(strings.ToLower(artifactId), "-", "."),
		"org." + strings.ToLower(artifactId),
		"com." + strings.ToLower(artifactId),
	}

	// Also try extracting groupId from artifactId if it contains dots
	if strings.Contains(artifactId, ".") {
		parts := strings.Split(artifactId, ".")
		if len(parts) > 1 {
			groupIds = append([]string{strings.Join(parts[:len(parts)-1], ".")}, groupIds...)
		}
	}

	var lastErr error
	var lastURL string
	for _, groupId := range groupIds {
		// Normalize groupId for URL (replace dots with slashes)
		groupIdPath := strings.ReplaceAll(groupId, ".", "/")
		urlStr := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
			groupIdPath, artifactId, version, artifactId, version)
		lastURL = urlStr
		fmt.Fprintf(os.Stderr, "    Trying POM URL: %s\n", urlStr)

		resp, err := s.client.Get(urlStr)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Fprintf(os.Stderr, "    ✓ Found POM at: %s\n", urlStr)
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				lastErr = fmt.Errorf("failed to read response: %w", err)
				continue
			}
			return data, groupId, nil
		}

		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// If direct groupId guessing failed, try Maven Central search API
	fmt.Fprintf(os.Stderr, "    All direct POM URLs failed, trying Maven Central search API...\n")
	pom, groupId, err := s.searchForPOM(artifactId, version)
	if err == nil {
		fmt.Fprintf(os.Stderr, "    ✓ Found POM via search API, groupId: %s\n", groupId)
		return pom, groupId, nil
	}

	return nil, "", fmt.Errorf("failed to fetch POM from %s (HTTP %v)", lastURL, lastErr)
}

// searchForPOM searches Maven Central for any POM.
func (s *MavenService) searchForPOM(artifactId, version string) ([]byte, string, error) {
	searchURL := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=a:%s&rows=20", url.QueryEscape(artifactId))

	resp, err := s.client.Get(searchURL)
	if err != nil {
		return nil, "", fmt.Errorf("search API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("search API returned HTTP %d", resp.StatusCode)
	}

	var searchResp MavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse search response: %w", err)
	}

	type doc struct {
		GroupID    string
		ArtifactID string
		Version    string
	}
	var matchingDocs []doc
	for _, d := range searchResp.Response.Docs {
		if d.ArtifactID == artifactId {
			matchingDocs = append(matchingDocs, doc{
				GroupID:    d.GroupID,
				ArtifactID: d.ArtifactID,
				Version:    d.Version,
			})
		}
	}

	sort.Slice(matchingDocs, func(i, j int) bool {
		return matchingDocs[i].GroupID < matchingDocs[j].GroupID
	})

	for _, doc := range matchingDocs {
		groupIdPath := strings.ReplaceAll(doc.GroupID, ".", "/")
		pomURL := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
			groupIdPath, artifactId, version, artifactId, version)

		pomResp, err := s.client.Get(pomURL)
		if err != nil {
			continue
		}
		defer pomResp.Body.Close()

		if pomResp.StatusCode != http.StatusOK {
			continue
		}

		pomData, err := io.ReadAll(pomResp.Body)
		if err != nil {
			continue
		}

		return pomData, doc.GroupID, nil
	}

	return nil, "", fmt.Errorf("no POM found in search results")
}

// FetchPOMByCoords fetches a POM using exact Maven coordinates.
func (s *MavenService) FetchPOMByCoords(groupId, artifactId, version string) ([]byte, error) {
	groupIdPath := strings.ReplaceAll(groupId, ".", "/")
	urlStr := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
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

// ResolveRepo implements the RepoResolver interface by fetching a POM, parsing it, and extracting the GitHub repo.
func (s *MavenService) ResolveRepo(groupId, artifactId, version string) (owner, repo string, err error) {
	// Fetch POM
	var pomData []byte
	if groupId != "" {
		pomData, err = s.FetchPOMByCoords(groupId, artifactId, version)
	} else {
		pomData, groupId, err = s.FetchPOM(artifactId, version)
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch POM: %w", err)
	}

	// Parse POM
	if s.pomParser == nil {
		return "", "", fmt.Errorf("pomParser is required for ResolveRepo")
	}
	pomStruct, err := s.pomParser.Parse(pomData)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse POM: %w", err)
	}

	// Extract from SCM
	if pomStruct.SCM.URL != "" || pomStruct.SCM.Connection != "" {
		scmInfo := &pomStruct.SCM
		if scmInfo.URL == "" {
			scmInfo.URL = scmInfo.Connection
		}
		owner, repo, err = ExtractGitHubRepoFromSCM(scmInfo)
		if err == nil {
			return owner, repo, nil
		}
	}

	// Extract from POM URL field
	if pomStruct.URL != "" {
		owner, repo, err = ExtractGitHubRepoFromURL(pomStruct.URL)
		if err == nil {
			return owner, repo, nil
		}
	}

	return "", "", fmt.Errorf("no GitHub repository found in POM")
}

type DepsDevService struct {
	client HTTPClient
}

func NewDepsDevService(client HTTPClient) *DepsDevService {
	if client == nil {
		client = NewHTTPClient()
	}
	return &DepsDevService{client: client}
}

// DepsDevVersionResponse represents the response from deps.dev API version endpoint.
type DepsDevVersionResponse struct {
	Links []struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"links"`
}

// ResolveRepo implements the RepoResolver interface by querying deps.dev API to find GitHub repository.
func (s *DepsDevService) ResolveRepo(groupId, artifactId, version string) (owner, repo string, err error) {
	packageKey := fmt.Sprintf("%s:%s", groupId, artifactId)
	endpoints := []string{
		fmt.Sprintf("https://api.deps.dev/v3/systems/maven/packages/%s/versions/%s", url.PathEscape(packageKey), url.PathEscape(version)),
		fmt.Sprintf("https://api.deps.dev/v3alpha/systems/maven/packages/%s/versions/%s", url.PathEscape(packageKey), url.PathEscape(version)),
	}

	fmt.Fprintf(os.Stderr, "Querying deps.dev for groupId=%s, artifactId=%s, version=%s\n", groupId, artifactId, version)
	fmt.Fprintf(os.Stderr, "  API endpoints:\n")
	for _, endpoint := range endpoints {
		fmt.Fprintf(os.Stderr, "    - %s\n", endpoint)
	}

	var lastErr error
	for _, depsDevURL := range endpoints {
		depsResp, err := s.client.Get(depsDevURL)
		if err != nil {
			lastErr = fmt.Errorf("deps.dev API request failed: %w", err)
			continue
		}
		defer depsResp.Body.Close()

		if depsResp.StatusCode == http.StatusOK {
			var depsDevResp DepsDevVersionResponse
			if err := json.NewDecoder(depsResp.Body).Decode(&depsDevResp); err != nil {
				lastErr = fmt.Errorf("failed to parse deps.dev response: %w", err)
				continue
			}

			for _, link := range depsDevResp.Links {
				if strings.Contains(link.URL, "github.com") {
					owner, repo, err := ExtractGitHubRepoFromURL(link.URL)
					if err == nil {
						return owner, repo, nil
					}
				}
			}
			lastErr = fmt.Errorf("no GitHub repository found in deps.dev version links")
		} else if depsResp.StatusCode != http.StatusNotFound {
			lastErr = fmt.Errorf("deps.dev API returned HTTP %d", depsResp.StatusCode)
		}
	}

	// HTML fallback
	htmlURL := fmt.Sprintf("https://deps.dev/maven/%s/%s/%s", strings.ReplaceAll(groupId, ".", "/"), artifactId, version)
	fmt.Fprintf(os.Stderr, "  HTML fallback: %s\n", htmlURL)
	htmlResp, err := s.client.Get(htmlURL)
	if err == nil {
		defer htmlResp.Body.Close()
		if htmlResp.StatusCode == http.StatusOK {
			htmlData, err := io.ReadAll(htmlResp.Body)
			if err == nil {
				htmlStr := string(htmlData)
				patterns := []*regexp.Regexp{
					regexp.MustCompile(`"url"\s*:\s*"https?://github\.com/([^/]+)/([^/"]+)"`),
					regexp.MustCompile(`href=["']https?://github\.com/([^/]+)/([^/"']+)(?:\.git)?["']`),
					regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:["'\s>]|\.git|/issues|/releases)`),
				}

				for _, pattern := range patterns {
					matches := pattern.FindStringSubmatch(htmlStr)
					if len(matches) >= 3 {
						owner = strings.TrimSpace(matches[1])
						repo = strings.TrimSpace(strings.TrimSuffix(matches[2], ".git"))
						if owner != "" && repo != "" && owner != "/" && repo != "/" && !strings.Contains(owner, " ") && !strings.Contains(repo, " ") {
							return owner, repo, nil
						}
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("deps.dev query failed: %v", lastErr)
}
