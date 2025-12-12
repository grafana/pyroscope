package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	// githubRepoPatterns are compiled regex patterns for extracting GitHub owner/repo from URLs.
	githubRepoPatterns = []*regexp.Regexp{
		regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`),
		regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+)`),
	}
)

func ExtractGitHubRepoFromURL(urlStr string) (owner, repo string, err error) {
	if !strings.Contains(urlStr, "github.com") {
		return "", "", fmt.Errorf("URL does not contain github.com: %s", urlStr)
	}

	for _, pattern := range githubRepoPatterns {
		matches := pattern.FindStringSubmatch(urlStr)
		if len(matches) >= 3 {
			owner = strings.TrimSpace(matches[1])
			repo = strings.TrimSpace(strings.TrimSuffix(matches[2], ".git"))
			if owner != "" && repo != "" && owner != "/" && repo != "/" {
				return owner, repo, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not extract GitHub repo from URL: %s", urlStr)
}

// RepoResolver is an interface for resolving GitHub repositories from Maven coordinates.
type RepoResolver interface {
	ResolveRepo(groupId, artifactId, version string) (owner, repo string, err error)
}

// GitHubResolver handles resolving GitHub repositories from Maven coordinates.
// It uses a simple strategy: deps.dev API first, then hardcoded jar-mappings.
type GitHubResolver struct {
	depsDevService *DepsDevService
	configService  *ConfigService
}

func NewGitHubResolver(depsDevService *DepsDevService, configService *ConfigService) *GitHubResolver {
	return &GitHubResolver{
		depsDevService: depsDevService,
		configService:  configService,
	}
}

// ResolveRepo resolves a GitHub repository using Maven coordinates.
// Strategy:
// 1. Check hardcoded jar-mappings.json first (for known edge cases)
// 2. Query deps.dev API (primary external source)
func (r *GitHubResolver) ResolveRepo(coords *MavenCoordinates) (owner, repo string, err error) {
	// Strategy 1: Check hardcoded mapping first
	if r.configService != nil {
		jarMapping := r.configService.FindJarMapping(coords.ArtifactID)
		if jarMapping != nil {
			fmt.Fprintf(os.Stderr, "  Found hardcoded mapping: %s/%s\n", jarMapping.Owner, jarMapping.Repo)
			return jarMapping.Owner, jarMapping.Repo, nil
		}
	}

	// Strategy 2: Query deps.dev API
	if r.depsDevService != nil && coords.GroupID != "" {
		fmt.Fprintf(os.Stderr, "  Querying deps.dev for %s:%s:%s\n", coords.GroupID, coords.ArtifactID, coords.Version)
		owner, repo, err = r.depsDevService.ResolveRepo(coords.GroupID, coords.ArtifactID, coords.Version)
		if err == nil {
			fmt.Fprintf(os.Stderr, "  Successfully resolved via deps.dev: %s/%s\n", owner, repo)
			return owner, repo, nil
		}
		fmt.Fprintf(os.Stderr, "  deps.dev lookup failed: %v\n", err)
	}

	// No resolution found
	if coords.GroupID == "" {
		return "", "", fmt.Errorf("cannot resolve GitHub repo: groupId unknown for %s:%s", coords.ArtifactID, coords.Version)
	}
	return "", "", fmt.Errorf("cannot resolve GitHub repo for %s:%s:%s", coords.GroupID, coords.ArtifactID, coords.Version)
}
