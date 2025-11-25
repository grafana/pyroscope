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

func extractGitHubRepoFromURLString(urlStr string) (owner, repo string, err error) {
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

func ExtractGitHubRepoFromURL(urlStr string) (owner, repo string, err error) {
	return extractGitHubRepoFromURLString(urlStr)
}

func ExtractGitHubRepoFromSCM(scm *SCM) (owner, repo string, err error) {
	url := strings.TrimSpace(scm.URL)
	if url == "" {
		url = strings.TrimSpace(scm.Connection)
	}
	return extractGitHubRepoFromURLString(url)
}

// GitHubResolver handles resolving GitHub repositories from JAR metadata.
type GitHubResolver struct {
	resolvers    []RepoResolver
	mavenService *MavenService // Kept for tryParentPOM which needs FetchPOMByCoords
	pomParser    *POMParser
}

func NewGitHubResolver(resolvers []RepoResolver, mavenService *MavenService, pomParser *POMParser) *GitHubResolver {
	return &GitHubResolver{
		resolvers:    resolvers,
		mavenService: mavenService,
		pomParser:    pomParser,
	}
}

// ResolveRepo resolves a GitHub repository using a strategy pattern.
func (r *GitHubResolver) ResolveRepo(jarMapping *JarMapping, pomStruct *POM, groupId, artifactId, version string) (owner, repo string, err error) {
	// Strategy 1: Check hardcoded mapping first
	if jarMapping != nil {
		fmt.Fprintf(os.Stderr, "  Found hardcoded mapping: %s/%s\n", jarMapping.Owner, jarMapping.Repo)
		return jarMapping.Owner, jarMapping.Repo, nil
	}

	// Strategy 2: Try SCM from POM
	if pomStruct.SCM.URL != "" || pomStruct.SCM.Connection != "" {
		fmt.Fprintf(os.Stderr, "  POM has SCM info: URL=%s, Connection=%s\n", pomStruct.SCM.URL, pomStruct.SCM.Connection)
		scmInfo := &pomStruct.SCM
		if scmInfo.URL == "" {
			scmInfo.URL = scmInfo.Connection
		}
		owner, repo, err = ExtractGitHubRepoFromSCM(scmInfo)
		if err == nil {
			fmt.Fprintf(os.Stderr, "  Successfully extracted GitHub repo from SCM: %s/%s\n", owner, repo)
			return owner, repo, nil
		}
		fmt.Fprintf(os.Stderr, "  SCM URL is not GitHub, trying POM URL field...\n")
		if pomStruct.URL != "" {
			fmt.Fprintf(os.Stderr, "  POM URL field: %s\n", pomStruct.URL)
			owner, repo, err = ExtractGitHubRepoFromURL(pomStruct.URL)
			if err == nil {
				return owner, repo, nil
			}
		}
		if err != nil && groupId != "" {
			fmt.Fprintf(os.Stderr, "  Falling back to resolvers...\n")
			for _, resolver := range r.resolvers {
				owner, repo, err = resolver.ResolveRepo(groupId, artifactId, version)
				if err == nil {
					fmt.Fprintf(os.Stderr, "  Successfully resolved via %T: %s/%s\n", resolver, owner, repo)
					return owner, repo, nil
				}
				fmt.Fprintf(os.Stderr, "  %T failed: %v\n", resolver, err)
			}
			return "", "", fmt.Errorf("invalid GitHub URL format (%s) and all resolvers failed", scmInfo.URL)
		} else if err != nil {
			if groupId == "" {
				return "", "", fmt.Errorf("invalid GitHub URL format (%s) and could not extract groupId for resolver query", scmInfo.URL)
			}
			return "", "", fmt.Errorf("invalid GitHub URL format (%s) and all resolvers failed", scmInfo.URL)
		}
	}

	// Strategy 3: No SCM, try parent POM first
	fmt.Fprintf(os.Stderr, "  POM has no SCM info, trying parent POM...\n")
	if pomStruct.Parent.GroupID != "" && pomStruct.Parent.ArtifactID != "" && pomStruct.Parent.Version != "" {
		fmt.Fprintf(os.Stderr, "  Parent POM: %s:%s:%s\n", pomStruct.Parent.GroupID, pomStruct.Parent.ArtifactID, pomStruct.Parent.Version)
		owner, repo, err = r.tryParentPOM(&pomStruct.Parent)
		if err == nil {
			return owner, repo, nil
		}
	}

	// Strategy 4: If parent didn't work, try POM URL field
	if err != nil && pomStruct.URL != "" {
		fmt.Fprintf(os.Stderr, "  Trying POM URL field: %s\n", pomStruct.URL)
		owner, repo, err = ExtractGitHubRepoFromURL(pomStruct.URL)
		if err == nil {
			return owner, repo, nil
		}
	}

	// Strategy 5: Fallback to resolvers
	if err != nil && groupId != "" {
		fmt.Fprintf(os.Stderr, "  Falling back to resolvers...\n")
		for _, resolver := range r.resolvers {
			owner, repo, err = resolver.ResolveRepo(groupId, artifactId, version)
			if err == nil {
				fmt.Fprintf(os.Stderr, "  Successfully resolved via %T: %s/%s\n", resolver, owner, repo)
				return owner, repo, nil
			}
			fmt.Fprintf(os.Stderr, "  %T failed: %v\n", resolver, err)
		}
		return "", "", fmt.Errorf("POM missing SCM information and all resolvers failed: %w", err)
	} else if err != nil {
		if groupId == "" {
			return "", "", fmt.Errorf("POM missing SCM information and could not extract groupId for resolver query")
		}
		return "", "", fmt.Errorf("POM missing SCM information and all resolvers failed: %w", err)
	}

	return "", "", fmt.Errorf("failed to resolve GitHub repository")
}

// tryParentPOM attempts to resolve GitHub repo from parent POM, including grandparent if needed.
func (r *GitHubResolver) tryParentPOM(parent *Parent) (owner, repo string, err error) {
	parentPOM, err := r.mavenService.FetchPOMByCoords(parent.GroupID, parent.ArtifactID, parent.Version)
	if err != nil {
		return "", "", err
	}

	parentPOMStruct, err := r.pomParser.Parse(parentPOM)
	if err != nil {
		return "", "", err
	}

	if parentPOMStruct.SCM.URL != "" || parentPOMStruct.SCM.Connection != "" {
		scmInfo := &parentPOMStruct.SCM
		if scmInfo.URL == "" {
			scmInfo.URL = scmInfo.Connection
		}
		owner, repo, err = ExtractGitHubRepoFromSCM(scmInfo)
		if err == nil {
			return owner, repo, nil
		}
	}

	if err != nil && parentPOMStruct.URL != "" {
		owner, repo, err = ExtractGitHubRepoFromURL(parentPOMStruct.URL)
		if err == nil {
			return owner, repo, nil
		}
	}

	if err != nil && parentPOMStruct.Parent.GroupID != "" && parentPOMStruct.Parent.ArtifactID != "" && parentPOMStruct.Parent.Version != "" {
		grandParentPOM, grandParentErr := r.mavenService.FetchPOMByCoords(parentPOMStruct.Parent.GroupID, parentPOMStruct.Parent.ArtifactID, parentPOMStruct.Parent.Version)
		if grandParentErr == nil {
			grandParentPOMStruct, err := r.pomParser.Parse(grandParentPOM)
			if err != nil {
				return "", "", err
			}
			if grandParentPOMStruct.SCM.URL != "" || grandParentPOMStruct.SCM.Connection != "" {
				scmInfo := &grandParentPOMStruct.SCM
				if scmInfo.URL == "" {
					scmInfo.URL = scmInfo.Connection
				}
				owner, repo, err = ExtractGitHubRepoFromSCM(scmInfo)
				if err == nil {
					return owner, repo, nil
				}
			}
			if err != nil && grandParentPOMStruct.URL != "" {
				owner, repo, err = ExtractGitHubRepoFromURL(grandParentPOMStruct.URL)
				if err == nil {
					return owner, repo, nil
				}
			}
		}
	}

	return "", "", err
}
