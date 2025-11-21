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
		jarPath = flag.String("jar", "", "Path to the Java JAR file to analyze")
		output  = flag.String("output", "", "Output file path (default: stdout)")
		help    = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help || *jarPath == "" {
		fmt.Println("Java JAR Source Code Mapper")
		fmt.Println()
		fmt.Println("Generates .pyroscope.yaml source_code mappings for 3rd party libraries")
		fmt.Println("found in a Java JAR file.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s -jar <jar-file> [-output <output-file>]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		return
	}

	if err := processJAR(*jarPath, *output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func processJAR(jarPath, outputPath string) error {
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

		mapping, err := processThirdPartyJAR(jarFile)
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

func processThirdPartyJAR(jarPath string) (*config.MappingConfig, error) {
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

	// Query Maven Central
	pom, err := fetchPOMFromMavenCentral(artifactId, version)
	if err != nil {
		return nil, err
	}

	// Parse POM for SCM info
	scmInfo, err := parseSCMFromPOM(pom)
	if err != nil {
		return nil, fmt.Errorf("POM missing SCM information: %w", err)
	}

	// Extract GitHub repo from SCM
	owner, repo, err := extractGitHubRepo(scmInfo)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub URL format (%s): %w", scmInfo.URL, err)
	}

	// Determine ref (use version from manifest)
	ref := version
	if strings.HasPrefix(ref, "v") {
		// Already has v prefix
	} else {
		// Try adding v prefix
		ref = "v" + ref
	}

	// Source path (hardcoded for MVP)
	sourcePath := "src/main/java"

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

	// Remove prefixes that are substrings of longer prefixes
	var filtered []string
	for _, prefix := range commonPrefixes {
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

func fetchPOMFromMavenCentral(artifactId, version string) ([]byte, error) {
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
			return data, nil
		}

		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// If direct groupId guessing failed, try Maven Central search API
	pom, err := searchMavenCentralForPOM(artifactId, version)
	if err == nil {
		return pom, nil
	}

	return nil, fmt.Errorf("failed to fetch POM from %s (HTTP %v)", lastURL, lastErr)
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

type POM struct {
	XMLName xml.Name `xml:"project"`
	SCM     SCM      `xml:"scm"`
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
