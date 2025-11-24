package main

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		jarPath     = flag.String("jar", "", "Path to the Java JAR file to analyze")
		output      = flag.String("output", "", "Output file path (default: stdout)")
		useMacaron  = flag.Bool("use-macaron", false, "Enable Macaron fallback when mapping fails")
		help        = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help || *jarPath == "" {
		fmt.Println("Java JAR Source Code Mapper")
		fmt.Println()
		fmt.Println("Generates .pyroscope.yaml source_code mappings for 3rd party libraries")
		fmt.Println("found in a Java JAR file.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s -jar <jar-file> [-output <output-file>] [-use-macaron]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		return
	}

	if err := processJAR(*jarPath, *output, *useMacaron); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func processJAR(jarPath, outputPath string, useMacaron bool) error {
	// Extract 3rd party JARs
	thirdPartyJARs, tmpDir, err := extractThirdPartyJARs(jarPath)
	if err != nil {
		return fmt.Errorf("failed to extract 3rd party JARs: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("Found %d 3rd party JARs\n", len(thirdPartyJARs))

	var mappings []config.MappingConfig
	successCount := 0
	failCount := 0

	// Process each JAR
	for _, jarFile := range thirdPartyJARs {
		fmt.Printf("Processing JAR: %s\n", filepath.Base(jarFile))

		mapping, err := processThirdPartyJAR(jarFile, useMacaron)
		if err != nil {
			fmt.Printf("✗ Skipping %s: %v\n", filepath.Base(jarFile), err)
			failCount++
			continue
		}

		if mapping != nil {
			mappings = append(mappings, *mapping)
			fmt.Printf("✓ Successfully mapped %s to %s/%s\n",
				filepath.Base(jarFile),
				mapping.Source.GitHub.Owner,
				mapping.Source.GitHub.Repo)
			successCount++
		} else {
			failCount++
		}
	}

	fmt.Printf("\nProcessed %d JARs: %d successful, %d failed\n",
		len(thirdPartyJARs), successCount, failCount)

	// Generate YAML output
	cfg := config.PyroscopeConfig{
		SourceCode: config.SourceCodeConfig{
			Mappings: mappings,
		},
	}

	var output io.Writer = os.Stdout
	if outputPath != "" {
		file, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()
		output = file
	}

	encoder := yaml.NewEncoder(output)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	return nil
}

func extractThirdPartyJARs(jarPath string) ([]string, string, error) {
	cmd := exec.Command("jar", "-tf", jarPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to list JAR contents: %w", err)
	}

	jarPattern := regexp.MustCompile(`BOOT-INF/lib/.*\.jar$`)
	var jarFiles []string

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if jarPattern.MatchString(line) {
			jarFiles = append(jarFiles, line)
		}
	}

	// Extract JARs to temporary directory
	tmpDir, err := os.MkdirTemp("", "jar-mapper-*")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	var extractedJARs []string
	mainJAR, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, tmpDir, fmt.Errorf("failed to open JAR: %w", err)
	}
	defer mainJAR.Close()

	for _, jarFile := range jarFiles {
		for _, f := range mainJAR.File {
			if f.Name == jarFile {
				extractedPath := filepath.Join(tmpDir, filepath.Base(jarFile))
				if err := extractFile(f, extractedPath); err != nil {
					fmt.Printf("Warning: failed to extract %s: %v\n", jarFile, err)
					continue
				}
				extractedJARs = append(extractedJARs, extractedPath)
				break
			}
		}
	}

	return extractedJARs, tmpDir, nil
}

func extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

func processThirdPartyJAR(jarPath string, useMacaron bool) (*config.MappingConfig, error) {
	// Extract class names and find prefixes
	prefixes, err := extractClassPrefixes(jarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract class names: %w", err)
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no common prefixes found in class names")
	}

	// Extract manifest
	manifest, err := extractManifest(jarPath)
	if err != nil {
		return nil, fmt.Errorf("missing MANIFEST.MF: %w", err)
	}

	version, ok := manifest["Implementation-Version"]
	if !ok || version == "" {
		return nil, fmt.Errorf("missing Implementation-Version in manifest")
	}

	// Extract artifactId from filename (more reliable than Implementation-Title)
	baseName := filepath.Base(jarPath)
	baseName = strings.TrimSuffix(baseName, ".jar")
	// Try to remove version suffix (pattern: -X.Y.Z or -X.Y)
	// This is a simple heuristic: remove last hyphen-separated parts that look like versions
	artifactId := baseName
	parts := strings.Split(baseName, "-")
	if len(parts) > 1 {
		// Check if last part looks like a version (contains digits)
		lastPart := parts[len(parts)-1]
		if strings.ContainsAny(lastPart, "0123456789") {
			// Remove version parts
			artifactId = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	var owner, repo string
	var pom []byte
	var groupId string

	// Query Maven Central
	pom, groupId, err = fetchPOMFromMavenCentral(artifactId, version)
	if err != nil {
		// If POM fetching failed and Macaron is enabled, try Macaron with common groupId patterns
		if useMacaron {
			// Try common groupId patterns for Macaron
			commonGroupIds := generateCommonGroupIds(artifactId)
			for i, guessedGroupId := range commonGroupIds {
				// Only show output on the last attempt to reduce noise
				verbose := (i == len(commonGroupIds)-1)
				if macaronOwner, macaronRepo, macaronErr := queryMacaronForGitHubRepo(guessedGroupId, artifactId, version, verbose); macaronErr == nil {
					// Success! Use Macaron result
					owner, repo = macaronOwner, macaronRepo
					groupId = guessedGroupId
					// We still need to create a minimal POM structure for determineSourcePath
					// But we can proceed with the mapping
					pomStruct := POM{GroupID: groupId}
					sourcePath := determineSourcePath(artifactId, pomStruct, groupId)
					ref := version
					if !strings.HasPrefix(ref, "v") {
						ref = "v" + ref
					}
					mapping := &config.MappingConfig{
						FunctionName: make([]config.Match, len(prefixes)),
						Language:     "java",
						Source: config.Source{
							GitHub: &config.GitHubMappingConfig{
								Owner: owner,
								Repo:  repo,
								Ref:   ref,
								Path:  sourcePath,
							},
						},
					}
					for i, prefix := range prefixes {
						mapping.FunctionName[i] = config.Match{Prefix: prefix}
					}
					return mapping, nil
				}
			}
			// Macaron failed for all groupId guesses
			return nil, fmt.Errorf("failed to fetch POM and Macaron fallback failed for all groupId patterns: %w", err)
		}
		return nil, err
	}

	// If groupId is still empty, try to extract from POM
	if groupId == "" {
		groupId, _ = extractGroupIdFromPOM(pom)
	}

	// Parse POM for SCM info and URL
	var pomStruct POM
	if err := xml.Unmarshal(pom, &pomStruct); err != nil {
		return nil, fmt.Errorf("failed to parse POM: %w", err)
	}

	// Try SCM first
	if pomStruct.SCM.URL != "" || pomStruct.SCM.Connection != "" {
		scmInfo := &pomStruct.SCM
		if scmInfo.URL == "" {
			scmInfo.URL = scmInfo.Connection
		}
		var err error
		owner, repo, err = extractGitHubRepo(scmInfo)
		if err == nil {
			// Successfully extracted from SCM
		} else {
			// SCM exists but not GitHub, try POM URL field
			if pomStruct.URL != "" {
				owner, repo, err = extractGitHubRepoFromURL(pomStruct.URL)
			}
			// If still failed, try deps.dev
			if err != nil && groupId != "" {
				owner, repo, err = queryDepsDevForGitHubRepo(groupId, artifactId, version)
				if err != nil {
					// Try Macaron as final fallback
					if useMacaron {
						if macaronOwner, macaronRepo, macaronErr := queryMacaronForGitHubRepo(groupId, artifactId, version, true); macaronErr == nil {
							owner, repo = macaronOwner, macaronRepo
							err = nil
						} else {
							return nil, fmt.Errorf("invalid GitHub URL format (%s), deps.dev query failed, and Macaron fallback failed: %w (macaron: %v)", scmInfo.URL, err, macaronErr)
						}
					} else {
						return nil, fmt.Errorf("invalid GitHub URL format (%s) and deps.dev query failed", scmInfo.URL)
					}
				}
			} else if err != nil {
				// Try Macaron as final fallback if we have groupId
				if groupId != "" && useMacaron {
					if macaronOwner, macaronRepo, macaronErr := queryMacaronForGitHubRepo(groupId, artifactId, version, true); macaronErr == nil {
						owner, repo = macaronOwner, macaronRepo
						err = nil
					} else {
						return nil, fmt.Errorf("invalid GitHub URL format (%s), could not extract groupId for deps.dev query, and Macaron fallback failed: %v", scmInfo.URL, macaronErr)
					}
				} else if groupId == "" {
					return nil, fmt.Errorf("invalid GitHub URL format (%s) and could not extract groupId for deps.dev or Macaron query", scmInfo.URL)
				} else {
					return nil, fmt.Errorf("invalid GitHub URL format (%s) and could not extract groupId for deps.dev query", scmInfo.URL)
				}
			}
		}
	} else {
		// No SCM, try parent POM first
		var err error
		if pomStruct.Parent.GroupID != "" && pomStruct.Parent.ArtifactID != "" && pomStruct.Parent.Version != "" {
			parentPOM, parentErr := fetchPOMFromMavenCentralByCoords(pomStruct.Parent.GroupID, pomStruct.Parent.ArtifactID, pomStruct.Parent.Version)
			if parentErr == nil {
				var parentPOMStruct POM
				if xml.Unmarshal(parentPOM, &parentPOMStruct) == nil {
					if parentPOMStruct.SCM.URL != "" || parentPOMStruct.SCM.Connection != "" {
						scmInfo := &parentPOMStruct.SCM
						if scmInfo.URL == "" {
							scmInfo.URL = scmInfo.Connection
						}
						owner, repo, err = extractGitHubRepo(scmInfo)
						if err == nil {
							// Successfully extracted from parent SCM
						}
					}
					if err != nil && parentPOMStruct.URL != "" {
						owner, repo, err = extractGitHubRepoFromURL(parentPOMStruct.URL)
					}
					// If parent POM also has a parent, try that too (up to one level deep)
					if err != nil && parentPOMStruct.Parent.GroupID != "" && parentPOMStruct.Parent.ArtifactID != "" && parentPOMStruct.Parent.Version != "" {
						grandParentPOM, grandParentErr := fetchPOMFromMavenCentralByCoords(parentPOMStruct.Parent.GroupID, parentPOMStruct.Parent.ArtifactID, parentPOMStruct.Parent.Version)
						if grandParentErr == nil {
							var grandParentPOMStruct POM
							if xml.Unmarshal(grandParentPOM, &grandParentPOMStruct) == nil {
								if grandParentPOMStruct.SCM.URL != "" || grandParentPOMStruct.SCM.Connection != "" {
									scmInfo := &grandParentPOMStruct.SCM
									if scmInfo.URL == "" {
										scmInfo.URL = scmInfo.Connection
									}
									owner, repo, err = extractGitHubRepo(scmInfo)
									if err == nil {
										// Successfully extracted from grandparent SCM
									}
								}
								if err != nil && grandParentPOMStruct.URL != "" {
									owner, repo, err = extractGitHubRepoFromURL(grandParentPOMStruct.URL)
								}
							}
						}
					}
				}
			}
		}

		// If parent didn't work, try POM URL field
		if err != nil && pomStruct.URL != "" {
			owner, repo, err = extractGitHubRepoFromURL(pomStruct.URL)
		}
		// If still failed, try deps.dev
		if err != nil && groupId != "" {
			owner, repo, err = queryDepsDevForGitHubRepo(groupId, artifactId, version)
			if err != nil {
				// Try Macaron as final fallback
				if useMacaron {
					if macaronOwner, macaronRepo, macaronErr := queryMacaronForGitHubRepo(groupId, artifactId, version, true); macaronErr == nil {
						owner, repo = macaronOwner, macaronRepo
						err = nil
					} else {
						return nil, fmt.Errorf("POM missing SCM information, deps.dev query failed, and Macaron fallback failed: %w (macaron: %v)", err, macaronErr)
					}
				} else {
					return nil, fmt.Errorf("POM missing SCM information and deps.dev query failed: %w", err)
				}
			}
		} else if err != nil {
			// Try Macaron as final fallback if we have groupId
			if groupId != "" && useMacaron {
				if macaronOwner, macaronRepo, macaronErr := queryMacaronForGitHubRepo(groupId, artifactId, version, true); macaronErr == nil {
					owner, repo = macaronOwner, macaronRepo
					err = nil
				} else {
					return nil, fmt.Errorf("POM missing SCM information, could not extract groupId for deps.dev query, and Macaron fallback failed: %v", macaronErr)
				}
			} else if groupId == "" {
				return nil, fmt.Errorf("POM missing SCM information and could not extract groupId for deps.dev or Macaron query")
			} else {
				return nil, fmt.Errorf("POM missing SCM information and could not extract groupId for deps.dev query")
			}
		}
	}

	// Validate that we have valid owner and repo
	// If all previous methods failed, try Macaron as a fallback
	if owner == "" || repo == "" {
		if groupId != "" && useMacaron {
			var err error
			owner, repo, err = queryMacaronForGitHubRepo(groupId, artifactId, version, true)
			if err != nil {
				return nil, fmt.Errorf("failed to extract valid GitHub owner/repo (owner: %q, repo: %q) and Macaron fallback failed: %w", owner, repo, err)
			}
		} else if groupId == "" {
			return nil, fmt.Errorf("failed to extract valid GitHub owner/repo (owner: %q, repo: %q) and could not extract groupId for Macaron fallback", owner, repo)
		} else {
			return nil, fmt.Errorf("failed to extract valid GitHub owner/repo (owner: %q, repo: %q)", owner, repo)
		}
	}

	// Determine ref (use version from manifest)
	ref := version
	if strings.HasPrefix(ref, "v") {
		// Already has v prefix
	} else {
		// Try adding v prefix
		ref = "v" + ref
	}

	// Source path - detect if this is likely a multi-module project
	// Multi-module Maven projects typically have:
	// 1. A parent POM (indicates it's a child module)
	// 2. ArtifactId that suggests it's a module (contains hyphens, matches common patterns)
	// 3. GroupId that matches the parent (common pattern)
	sourcePath := determineSourcePath(artifactId, pomStruct, groupId)

	mapping := &config.MappingConfig{
		FunctionName: make([]config.Match, len(prefixes)),
		Language:     "java",
		Source: config.Source{
			GitHub: &config.GitHubMappingConfig{
				Owner: owner,
				Repo:  repo,
				Ref:   ref,
				Path:  sourcePath,
			},
		},
	}

	for i, prefix := range prefixes {
		mapping.FunctionName[i] = config.Match{Prefix: prefix}
	}

	return mapping, nil
}

// determineSourcePath determines the correct source path for a Java project
// It detects if this is a multi-module Maven project and includes the module name in the path
// This encapsulates all the logic for determining module paths that was previously in find_java.go
func determineSourcePath(artifactId string, pomStruct POM, groupId string) string {
	// Default path for single-module projects
	sourcePath := "src/main/java"

	// Multi-module Maven projects typically have:
	// 1. A parent POM (indicates it's a child module) - this is the strongest indicator
	// 2. ArtifactId that contains hyphens (common pattern for modules like spring-webmvc, jackson-databind)
	//
	// If either condition is true, use artifactId as the module prefix
	// This handles cases like:
	// - spring-webmvc (has parent, contains hyphen)
	// - spring-web (has parent, contains hyphen)
	// - jackson-databind (has parent, contains hyphen)
	hasParent := pomStruct.Parent.GroupID != ""
	looksLikeModule := strings.Contains(artifactId, "-") && len(artifactId) > 5

	if hasParent || looksLikeModule {
		sourcePath = filepath.Join(artifactId, "src/main/java")
	}

	return sourcePath
}

func extractClassPrefixes(jarPath string) ([]string, error) {
	cmd := exec.Command("jar", "-tf", jarPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	classPattern := regexp.MustCompile(`^([^/]+(/[^/]+)*)/[^/]+\.class$`)
	var packages []string

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if matches := classPattern.FindStringSubmatch(line); matches != nil {
			packages = append(packages, matches[1])
		}
	}

	if len(packages) == 0 {
		return nil, fmt.Errorf("no class files found")
	}

	// Find common prefixes
	prefixes := findCommonPrefixes(packages)
	return prefixes, nil
}

func findCommonPrefixes(packages []string) []string {
	if len(packages) == 0 {
		return nil
	}

	// Count occurrences of each prefix
	prefixCount := make(map[string]int)

	for _, pkg := range packages {
		parts := strings.Split(pkg, "/")
		for i := 1; i <= len(parts); i++ {
			prefix := strings.Join(parts[:i], "/")
			prefixCount[prefix]++
		}
	}

	// Find prefixes that appear in multiple packages (at least 2)
	var commonPrefixes []string
	seen := make(map[string]bool)

	for prefix, count := range prefixCount {
		if count >= 2 && !seen[prefix] {
			commonPrefixes = append(commonPrefixes, prefix)
			seen[prefix] = true
		}
	}

	// Sort by length (longest first) to prefer more specific prefixes
	for i := 0; i < len(commonPrefixes); i++ {
		for j := i + 1; j < len(commonPrefixes); j++ {
			if len(commonPrefixes[i]) < len(commonPrefixes[j]) {
				commonPrefixes[i], commonPrefixes[j] = commonPrefixes[j], commonPrefixes[i]
			}
		}
	}

	// Build a set of actual package names (not just prefixes)
	packageSet := make(map[string]bool)
	for _, pkg := range packages {
		packageSet[pkg] = true
	}

	// Remove prefixes that are substrings of longer prefixes
	// BUT keep prefixes that are actual packages (have classes directly in them)
	var filtered []string
	for _, prefix := range commonPrefixes {
		// If this prefix is an actual package (has classes directly in it), keep it
		if packageSet[prefix] {
			filtered = append(filtered, prefix)
			continue
		}

		// Otherwise, only keep it if it's not a substring of a longer prefix
		isSubstring := false
		for _, other := range commonPrefixes {
			if prefix != other && strings.HasPrefix(other, prefix+"/") {
				isSubstring = true
				break
			}
		}
		if !isSubstring {
			filtered = append(filtered, prefix)
		}
	}

	return filtered
}

func extractManifest(jarPath string) (map[string]string, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, f := range reader.File {
		if f.Name == "META-INF/MANIFEST.MF" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			return parseManifest(string(data)), nil
		}
	}

	return nil, fmt.Errorf("MANIFEST.MF not found")
}

func parseManifest(data string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(data, "\n")

	var currentKey string
	var currentValue strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			// Continuation line
			if currentKey != "" {
				currentValue.WriteString(" ")
				currentValue.WriteString(strings.TrimSpace(line))
			}
		} else {
			// New key-value pair
			if currentKey != "" {
				// Only store if we haven't seen this key before (take first occurrence)
				if _, exists := result[currentKey]; !exists {
					result[currentKey] = currentValue.String()
				}
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := parts[0]
				// If we see a "Name:" line, we're entering a named section
				// Skip keys in named sections if we already have them from main section
				if key == "Name" {
					// Reset current key to stop processing continuation lines
					currentKey = ""
					currentValue.Reset()
					continue
				}
				// Only process this key if we haven't seen it before
				if _, exists := result[key]; !exists {
					currentKey = key
					currentValue.Reset()
					currentValue.WriteString(strings.TrimSpace(parts[1]))
				} else {
					// Skip this key, we already have it
					currentKey = ""
					currentValue.Reset()
				}
			}
		}
	}

	if currentKey != "" {
		if _, exists := result[currentKey]; !exists {
			result[currentKey] = currentValue.String()
		}
	}

	return result
}

func fetchPOMFromMavenCentral(artifactId, version string) ([]byte, string, error) {
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

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	var lastURL string
	for _, groupId := range groupIds {
		// Normalize groupId for URL (replace dots with slashes)
		groupIdPath := strings.ReplaceAll(groupId, ".", "/")
		url := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
			groupIdPath, artifactId, version, artifactId, version)
		lastURL = url

		resp, err := client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
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
	// This will return a POM even if it doesn't have SCM (we'll use deps.dev for that)
	pom, groupId, err := searchMavenCentralForPOMAny(artifactId, version)
	if err == nil {
		return pom, groupId, nil
	}

	return nil, "", fmt.Errorf("failed to fetch POM from %s (HTTP %v)", lastURL, lastErr)
}

// generateCommonGroupIds generates common groupId patterns for a given artifactId
// This is used when POM fetching fails and we need to try Macaron with guessed groupIds
func generateCommonGroupIds(artifactId string) []string {
	groupIds := []string{
		strings.ToLower(artifactId),
		strings.ReplaceAll(strings.ToLower(artifactId), "-", "."),
		"org." + strings.ToLower(artifactId),
		"com." + strings.ToLower(artifactId),
	}

	// If artifactId contains dots, try using it as groupId prefix
	if strings.Contains(artifactId, ".") {
		parts := strings.Split(artifactId, ".")
		if len(parts) > 1 {
			groupIds = append([]string{strings.Join(parts[:len(parts)-1], ".")}, groupIds...)
		}
	}

	return groupIds
}

// MavenSearchResponse represents the response from Maven Central search API
type MavenSearchResponse struct {
	Response struct {
		Docs []struct {
			GroupID    string `json:"g"`
			ArtifactID string `json:"a"`
			Version    string `json:"latestVersion"`
		} `json:"docs"`
	} `json:"response"`
}

// searchMavenCentralForPOMAny searches Maven Central for any POM (not requiring SCM)
// Returns the POM data and the groupId from the search results
func searchMavenCentralForPOMAny(artifactId, version string) ([]byte, string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Search for artifactId
	searchURL := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=a:%s&rows=20", url.QueryEscape(artifactId))

	resp, err := client.Get(searchURL)
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

	// Try each result - fetch POM with exact version
	for _, doc := range searchResp.Response.Docs {
		// Fetch the POM for this groupId/artifactId/version
		groupIdPath := strings.ReplaceAll(doc.GroupID, ".", "/")
		pomURL := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
			groupIdPath, doc.ArtifactID, version, doc.ArtifactID, version)

		pomResp, err := client.Get(pomURL)
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

		// Return the first valid POM found along with the groupId from search results
		return pomData, doc.GroupID, nil
	}

	return nil, "", fmt.Errorf("no POM found in search results")
}

// fetchPOMFromMavenCentralByCoords fetches a POM using exact Maven coordinates
func fetchPOMFromMavenCentralByCoords(groupId, artifactId, version string) ([]byte, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	groupIdPath := strings.ReplaceAll(groupId, ".", "/")
	url := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
		groupIdPath, artifactId, version, artifactId, version)

	resp, err := client.Get(url)
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

// searchMavenCentralForPOM searches Maven Central for a POM with SCM information
func searchMavenCentralForPOM(artifactId, version string) ([]byte, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Search for artifactId
	searchURL := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=a:%s&rows=20", url.QueryEscape(artifactId))

	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("search API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search API returned HTTP %d", resp.StatusCode)
	}

	var searchResp MavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	// Try each result - fetch POM with exact version
	for _, doc := range searchResp.Response.Docs {
		// Fetch the POM for this groupId/artifactId/version
		groupIdPath := strings.ReplaceAll(doc.GroupID, ".", "/")
		pomURL := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
			groupIdPath, doc.ArtifactID, version, doc.ArtifactID, version)

		pomResp, err := client.Get(pomURL)
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

		// Check if this POM has SCM information
		scmInfo, err := parseSCMFromPOM(pomData)
		if err == nil && scmInfo != nil {
			// Found a POM with SCM info, return it
			return pomData, nil
		}
	}

	return nil, fmt.Errorf("no POM with SCM information found in search results")
}

// DepsDevVersionResponse represents the response from deps.dev API version endpoint
type DepsDevVersionResponse struct {
	Links []struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"links"`
}

// queryDepsDevForGitHubRepo queries deps.dev API to find GitHub repository
func queryDepsDevForGitHubRepo(groupId, artifactId, version string) (owner, repo string, err error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Use the correct deps.dev API endpoint format: /v3/systems/{system}/packages/{package}/versions/{version}
	packageKey := fmt.Sprintf("%s:%s", groupId, artifactId)
	endpoints := []string{
		fmt.Sprintf("https://api.deps.dev/v3/systems/maven/packages/%s/versions/%s", url.PathEscape(packageKey), url.PathEscape(version)),
		fmt.Sprintf("https://api.deps.dev/v3alpha/systems/maven/packages/%s/versions/%s", url.PathEscape(packageKey), url.PathEscape(version)),
	}

	var lastErr error
	for _, depsDevURL := range endpoints {
		depsResp, err := client.Get(depsDevURL)
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

			// Look for GitHub link in version links
			// Prefer SOURCE_REPO, then HOMEPAGE, then any link with github.com
			for _, link := range depsDevResp.Links {
				if strings.Contains(link.URL, "github.com") {
					// Extract GitHub repo from URL
					owner, repo, err := extractGitHubRepoFromURL(link.URL)
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

	// If all API endpoints failed, try fetching the HTML page and parsing it
	// This is a fallback for when the API is not available
	// Note: deps.dev uses React, so GitHub links may be in JSON data embedded in the page
	htmlURL := fmt.Sprintf("https://deps.dev/maven/%s/%s/%s", strings.ReplaceAll(groupId, ".", "/"), artifactId, version)
	htmlResp, err := client.Get(htmlURL)
	if err == nil {
		defer htmlResp.Body.Close()
		if htmlResp.StatusCode == http.StatusOK {
			htmlData, err := io.ReadAll(htmlResp.Body)
			if err == nil {
				htmlStr := string(htmlData)

				// Try multiple patterns to find GitHub URLs in the HTML
				// Look for embedded JSON data that might contain GitHub links
				patterns := []*regexp.Regexp{
					// JSON format: "url": "https://github.com/owner/repo"
					regexp.MustCompile(`"url"\s*:\s*"https?://github\.com/([^/]+)/([^/"]+)"`),
					// href attributes
					regexp.MustCompile(`href=["']https?://github\.com/([^/]+)/([^/"']+)(?:\.git)?["']`),
					// General GitHub URL patterns
					regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:["'\s>]|\.git|/issues|/releases)`),
				}

				for _, pattern := range patterns {
					matches := pattern.FindStringSubmatch(htmlStr)
					if len(matches) >= 3 {
						owner = strings.TrimSpace(matches[1])
						repo = strings.TrimSpace(strings.TrimSuffix(matches[2], ".git"))
						// Basic validation: owner and repo should be non-empty and valid
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

// MacaronSourceResponse represents the response from Macaron find-source command
// Macaron may output JSON or structured text, so we'll try to parse both
type MacaronSourceResponse struct {
	Repository string `json:"repository"`
	URL        string `json:"url"`
	Source     struct {
		Repository string `json:"repository"`
		URL        string `json:"url"`
	} `json:"source"`
}

// findMacaronScript finds the Macaron script in various locations
func findMacaronScript() (string, error) {
	// First, check environment variable
	if envPath := os.Getenv("MACARON_SCRIPT"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
	}

	// Try current directory and relative paths
	possiblePaths := []string{
		"./run_macaron.sh",
		"run_macaron.sh",
		"../run_macaron.sh",
		"../../run_macaron.sh",
	}
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try to find in PATH
	if path, err := exec.LookPath("run_macaron.sh"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("macaron"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("macaron script not found (checked: ./run_macaron.sh, relative paths, PATH, and MACARON_SCRIPT env var). Set MACARON_SCRIPT environment variable to the path of run_macaron.sh or ensure it's in PATH")
}

// queryMacaronForGitHubRepo queries Macaron find-source command to find GitHub repository
// verbose controls whether to print Macaron output to stderr
func queryMacaronForGitHubRepo(groupId, artifactId, version string, verbose bool) (owner, repo string, err error) {
	// Construct PURL: pkg:maven/<groupId>/<artifactId>@<version>
	purl := fmt.Sprintf("pkg:maven/%s/%s@%s", groupId, artifactId, version)

	// Find Macaron script
	macaronScript, err := findMacaronScript()
	if err != nil {
		return "", "", err
	}

	// Execute Macaron find-source command
	cmd := exec.Command(macaronScript, "find-source", "-purl", purl)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Print Macaron output for debugging (only if verbose)
	if verbose {
		fmt.Fprintf(os.Stderr, "Macaron output for %s:\n%s\n", purl, outputStr)
	}

	// Check if command failed (non-zero exit code)
	// Warnings don't necessarily mean failure, so we'll try to parse output anyway
	if err != nil {
		// If there's an error, check if we can still extract useful info from output
		// Some commands output warnings to stderr but still produce valid output
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() != 0 {
			// Non-zero exit code - try to parse anyway in case there's useful info
			// but if parsing fails, return the error
		} else {
			// Other error (like command not found)
			return "", "", fmt.Errorf("macaron command failed: %w (output: %s)", err, outputStr)
		}
	}

	// Filter out warning lines for cleaner parsing
	// Macaron may output warnings like "[WARNING]: ..." that we should ignore
	lines := strings.Split(outputStr, "\n")
	var filteredLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and warning lines
		if line == "" {
			continue
		}
		// Skip common warning patterns
		if strings.HasPrefix(line, "[WARNING]:") || 
		   strings.HasPrefix(line, "[ERROR]:") ||
		   strings.HasPrefix(line, "WARNING:") ||
		   strings.HasPrefix(line, "ERROR:") {
			continue
		}
		filteredLines = append(filteredLines, line)
	}
	// Reconstruct output without warnings
	outputStr = strings.Join(filteredLines, "\n")

	// Try to parse as JSON first (use filtered output)
	if outputStr != "" {
		var macaronResp MacaronSourceResponse
		if jsonErr := json.Unmarshal([]byte(outputStr), &macaronResp); jsonErr == nil {
			// Try to extract from JSON structure
			repoURL := macaronResp.Repository
			if repoURL == "" {
				repoURL = macaronResp.URL
			}
			if repoURL == "" {
				repoURL = macaronResp.Source.Repository
				if repoURL == "" {
					repoURL = macaronResp.Source.URL
				}
			}
			if repoURL != "" {
				return extractGitHubRepoFromURL(repoURL)
			}
		}
	}

	// Also try parsing the original output (before filtering) in case JSON spans multiple lines
	if len(output) > 0 {
		var macaronResp MacaronSourceResponse
		if jsonErr := json.Unmarshal(output, &macaronResp); jsonErr == nil {
			repoURL := macaronResp.Repository
			if repoURL == "" {
				repoURL = macaronResp.URL
			}
			if repoURL == "" {
				repoURL = macaronResp.Source.Repository
				if repoURL == "" {
					repoURL = macaronResp.Source.URL
				}
			}
			if repoURL != "" {
				return extractGitHubRepoFromURL(repoURL)
			}
		}
	}

	// If JSON parsing failed or didn't contain repo info, try text parsing
	// Look for GitHub URLs in the output
	patterns := []*regexp.Regexp{
		// Direct GitHub URL patterns (most specific first)
		regexp.MustCompile(`https?://github\.com/([^/\s"']+)/([^/\s"']+?)(?:\.git)?(?:/|"|'|\s|$)`),
		regexp.MustCompile(`github\.com[:/]([^/\s"']+)/([^/\s"']+?)(?:\.git)?(?:/|"|'|\s|$)`),
		// Repository field patterns
		regexp.MustCompile(`(?i)repository["\s:]+(?:https?://)?github\.com/([^/\s"']+)/([^/\s"']+?)(?:\.git)?`),
		regexp.MustCompile(`(?i)url["\s:]+(?:https?://)?github\.com/([^/\s"']+)/([^/\s"']+?)(?:\.git)?`),
		// Source repository patterns
		regexp.MustCompile(`(?i)source["\s:]+(?:https?://)?github\.com/([^/\s"']+)/([^/\s"']+?)(?:\.git)?`),
		// More general patterns
		regexp.MustCompile(`github\.com/([^/\s"']+)/([^/\s"']+?)(?:\.git)?`),
	}

	// Try filtered output first (without warnings)
	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(outputStr)
		if len(matches) >= 3 {
			owner = strings.TrimSpace(matches[1])
			repo = strings.TrimSpace(strings.TrimSuffix(matches[2], ".git"))
			// Validate that owner and repo are non-empty and valid
			if owner != "" && repo != "" && owner != "/" && repo != "/" && !strings.Contains(owner, " ") && !strings.Contains(repo, " ") {
				return owner, repo, nil
			}
		}
	}

	// If filtered output didn't work, try original output (in case URL is in a warning line)
	originalOutputStr := string(output)
	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(originalOutputStr)
		if len(matches) >= 3 {
			owner = strings.TrimSpace(matches[1])
			repo = strings.TrimSpace(strings.TrimSuffix(matches[2], ".git"))
			// Validate that owner and repo are non-empty and valid
			if owner != "" && repo != "" && owner != "/" && repo != "/" && !strings.Contains(owner, " ") && !strings.Contains(repo, " ") {
				return owner, repo, nil
			}
		}
	}

	// If we still couldn't find it, return error with truncated output (to avoid huge error messages)
	errorOutput := outputStr
	if errorOutput == "" {
		errorOutput = originalOutputStr
	}
	// Truncate if too long
	if len(errorOutput) > 500 {
		errorOutput = errorOutput[:500] + "... (truncated)"
	}
	return "", "", fmt.Errorf("could not extract GitHub repository from Macaron output: %s", errorOutput)
}

// extractGitHubRepoFromURL extracts owner and repo from a GitHub URL
func extractGitHubRepoFromURL(urlStr string) (owner, repo string, err error) {
	// Only process if URL contains github.com
	if !strings.Contains(urlStr, "github.com") {
		return "", "", fmt.Errorf("URL does not contain github.com: %s", urlStr)
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`),
		regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+)`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(urlStr)
		if len(matches) >= 3 {
			owner = strings.TrimSpace(matches[1])
			repo = strings.TrimSpace(strings.TrimSuffix(matches[2], ".git"))
			// Validate that owner and repo are non-empty and don't contain invalid characters
			if owner != "" && repo != "" && owner != "/" && repo != "/" {
				return owner, repo, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not extract GitHub repo from URL: %s", urlStr)
}

type POM struct {
	XMLName xml.Name `xml:"project"`
	GroupID string   `xml:"groupId"`
	URL     string   `xml:"url"`
	SCM     SCM      `xml:"scm"`
	Parent  Parent   `xml:"parent"`
}

type Parent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type SCM struct {
	URL        string `xml:"url"`
	Connection string `xml:"connection"`
	Tag        string `xml:"tag"`
}

func parseSCMFromPOM(pomData []byte) (*SCM, error) {
	var pom POM
	if err := xml.Unmarshal(pomData, &pom); err != nil {
		return nil, fmt.Errorf("invalid POM XML: %w", err)
	}

	if pom.SCM.URL == "" && pom.SCM.Connection == "" {
		return nil, fmt.Errorf("no SCM information found")
	}

	scm := &pom.SCM
	if scm.URL == "" {
		scm.URL = scm.Connection
	}

	return scm, nil
}

func extractGroupIdFromPOM(pomData []byte) (string, error) {
	var pom POM
	if err := xml.Unmarshal(pomData, &pom); err != nil {
		return "", fmt.Errorf("invalid POM XML: %w", err)
	}
	return pom.GroupID, nil
}

func extractGitHubRepo(scm *SCM) (owner, repo string, err error) {
	url := strings.TrimSpace(scm.URL)
	if url == "" {
		url = strings.TrimSpace(scm.Connection)
	}

	// Handle different URL formats
	// https://github.com/owner/repo
	// git@github.com:owner/repo.git
	// scm:git:git://github.com/owner/repo.git

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`),
		regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+)`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(url)
		if len(matches) >= 3 {
			owner = matches[1]
			repo = strings.TrimSuffix(matches[2], ".git")
			return owner, repo, nil
		}
	}

	return "", "", fmt.Errorf("could not extract GitHub repo from URL: %s", url)
}
