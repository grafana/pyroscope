package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type CommandRunner interface {
	RunCommand(name string, args ...string) ([]byte, error)
}

type DefaultCommandRunner struct{}

func (r *DefaultCommandRunner) RunCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.Output()
}

type JARAnalyzer struct {
	runner CommandRunner
}

func NewJARAnalyzer() *JARAnalyzer {
	return &JARAnalyzer{
		runner: &DefaultCommandRunner{},
	}
}

func (a *JARAnalyzer) ExtractClassPrefixes(jarPath string) ([]string, error) {
	output, err := a.runner.RunCommand("jar", "-tf", jarPath)
	if err != nil {
		return nil, err
	}

	classPattern := regexp.MustCompile(`^([^/]+(/[^/]+)*)/[^/]+\.class$`)
	packageSet := make(map[string]bool)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if matches := classPattern.FindStringSubmatch(line); matches != nil {
			packageSet[matches[1]] = true
		}
	}

	if len(packageSet) == 0 {
		return nil, fmt.Errorf("no class files found")
	}

	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	prefixes := findCommonPrefixes(packages)
	return prefixes, nil
}

func findCommonPrefixes(packages []string) []string {
	if len(packages) == 0 {
		return nil
	}

	// Filter out shaded/vendor packages before counting
	var filteredPackages []string
	for _, pkg := range packages {
		if !isShadedPackage(pkg) {
			filteredPackages = append(filteredPackages, pkg)
		}
	}

	// If all packages were filtered out, fall back to original list
	if len(filteredPackages) == 0 {
		filteredPackages = packages
	}

	prefixCount := make(map[string]int)
	for _, pkg := range filteredPackages {
		parts := strings.Split(pkg, "/")
		for i := 1; i <= len(parts); i++ {
			prefix := strings.Join(parts[:i], "/")
			prefixCount[prefix]++
		}
	}

	prefixKeys := make([]string, 0, len(prefixCount))
	for prefix := range prefixCount {
		prefixKeys = append(prefixKeys, prefix)
	}
	sort.Strings(prefixKeys)

	var commonPrefixes []string
	seen := make(map[string]bool)
	for _, prefix := range prefixKeys {
		count := prefixCount[prefix]
		if count >= 2 && !seen[prefix] {
			commonPrefixes = append(commonPrefixes, prefix)
			seen[prefix] = true
		}
	}

	sort.Slice(commonPrefixes, func(i, j int) bool {
		lenI, lenJ := len(commonPrefixes[i]), len(commonPrefixes[j])
		if lenI != lenJ {
			return lenI > lenJ
		}
		return commonPrefixes[i] < commonPrefixes[j]
	})

	packageSet := make(map[string]bool)
	for _, pkg := range packages {
		packageSet[pkg] = true
	}

	var filtered []string
	for _, prefix := range commonPrefixes {
		if packageSet[prefix] {
			filtered = append(filtered, prefix)
			continue
		}

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

// isShadedPackage detects packages that are shaded/relocated dependencies.
// These are dependencies that have been relocated to a different package during the build
// and don't exist in the original source repository.
// Common patterns include:
//   - vendor/
//   - shaded/
//   - repackaged/
//   - internal/shaded/
//   - relocated/
func isShadedPackage(pkg string) bool {
	shadedPatterns := []string{
		"/vendor/",
		"/shaded/",
		"/repackaged/",
		"/relocated/",
		"/internal/shaded/",
		"/shadow/",
		"/thirdparty/",
	}

	pkgLower := strings.ToLower(pkg)
	for _, pattern := range shadedPatterns {
		if strings.Contains(pkgLower, pattern) {
			return true
		}
	}

	return false
}

func (a *JARAnalyzer) ExtractManifest(jarPath string) (map[string]string, error) {
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
			if currentKey != "" {
				currentValue.WriteString(" ")
				currentValue.WriteString(strings.TrimSpace(line))
			}
		} else {
			if currentKey != "" {
				if _, exists := result[currentKey]; !exists {
					result[currentKey] = currentValue.String()
				}
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := parts[0]
				if key == "Name" {
					currentKey = ""
					currentValue.Reset()
					continue
				}
				if _, exists := result[key]; !exists {
					currentKey = key
					currentValue.Reset()
					currentValue.WriteString(strings.TrimSpace(parts[1]))
				} else {
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

// MavenCoordinates holds Maven artifact coordinates extracted from pom.properties.
type MavenCoordinates struct {
	GroupID    string
	ArtifactID string
	Version    string
}

// ExtractPOMProperties extracts Maven coordinates from the embedded pom.properties file.
// Maven JARs typically contain META-INF/maven/{groupId}/{artifactId}/pom.properties.
// For shaded JARs that contain multiple pom.properties files, this function prefers
// the one whose artifactId matches the JAR filename.
func (a *JARAnalyzer) ExtractPOMProperties(jarPath string) (*MavenCoordinates, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Extract base name from JAR filename for matching
	baseName := filepath.Base(jarPath)
	baseName = strings.TrimSuffix(baseName, ".jar")
	// Extract artifactId from filename (remove version suffix like "-1.2.3")
	expectedArtifactId := extractArtifactIdFromFilename(baseName)

	// Collect all valid pom.properties
	pomPropsPattern := regexp.MustCompile(`^META-INF/maven/[^/]+/[^/]+/pom\.properties$`)
	var allCoords []*MavenCoordinates
	var matchingCoords *MavenCoordinates

	for _, f := range reader.File {
		if pomPropsPattern.MatchString(f.Name) {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			coords := parsePOMProperties(string(data))
			if coords.GroupID != "" && coords.ArtifactID != "" && coords.Version != "" {
				allCoords = append(allCoords, coords)
				// Check if this pom.properties matches the JAR filename
				if matchingCoords == nil && artifactIdMatchesFilename(coords.ArtifactID, expectedArtifactId, baseName) {
					matchingCoords = coords
				}
			}
		}
	}

	// Prefer the pom.properties that matches the JAR filename
	if matchingCoords != nil {
		return matchingCoords, nil
	}

	// If we have pom.properties but none match the filename, this is likely
	// a shaded JAR where the main artifact's metadata was stripped but
	// shaded dependency metadata remained. Don't use incorrect coordinates.
	if len(allCoords) > 0 {
		return nil, fmt.Errorf("pom.properties found but artifactId %q doesn't match JAR filename %q (likely shaded JAR)",
			allCoords[0].ArtifactID, baseName)
	}

	return nil, fmt.Errorf("no pom.properties found in JAR")
}

// extractArtifactIdFromFilename extracts the artifactId part from a JAR filename.
// E.g., "spring-web-5.3.20" -> "spring-web", "agent-2.1.2" -> "agent"
func extractArtifactIdFromFilename(baseName string) string {
	parts := strings.Split(baseName, "-")
	if len(parts) <= 1 {
		return baseName
	}
	// Find where version starts (first part that looks like a version number)
	for i := len(parts) - 1; i > 0; i-- {
		if looksLikeVersion(parts[i]) {
			return strings.Join(parts[:i], "-")
		}
	}
	return baseName
}

// looksLikeVersion checks if a string looks like a version number.
func looksLikeVersion(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Version typically starts with a digit
	return s[0] >= '0' && s[0] <= '9'
}

// artifactIdMatchesFilename checks if a pom.properties artifactId matches the JAR filename.
func artifactIdMatchesFilename(artifactId, expectedArtifactId, baseName string) bool {
	// Direct match
	if artifactId == expectedArtifactId {
		return true
	}
	// Check if baseName starts with artifactId (handles version suffix variations)
	if strings.HasPrefix(baseName, artifactId+"-") || baseName == artifactId {
		return true
	}
	return false
}

// parsePOMProperties parses a pom.properties file content.
func parsePOMProperties(data string) *MavenCoordinates {
	coords := &MavenCoordinates{}
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "groupId":
			coords.GroupID = value
		case "artifactId":
			coords.ArtifactID = value
		case "version":
			coords.Version = value
		}
	}
	return coords
}

// ExtractMavenCoordinates extracts Maven coordinates using multiple strategies:
// 1. pom.properties (most reliable for Maven-published JARs)
// 2. Bazel path parsing (for JARs in Bazel runfiles with Maven path structure)
// 3. MANIFEST.MF + filename parsing (fallback for other JARs)
func (a *JARAnalyzer) ExtractMavenCoordinates(jarPath string) (*MavenCoordinates, error) {
	// Try pom.properties first (most reliable)
	coords, err := a.ExtractPOMProperties(jarPath)
	if err == nil && coords.GroupID != "" && coords.ArtifactID != "" && coords.Version != "" {
		return coords, nil
	}

	// Try extracting from Bazel path (e.g., .../maven2/io/pyroscope/agent/2.1.2/...)
	coords = extractMavenCoordinatesFromPath(jarPath)
	if coords.GroupID != "" && coords.ArtifactID != "" && coords.Version != "" {
		return coords, nil
	}

	// Fallback to manifest + filename parsing
	artifactId, version, err := a.extractArtifactInfoFromManifest(jarPath)
	if err != nil {
		return nil, err
	}

	return &MavenCoordinates{
		ArtifactID: artifactId,
		Version:    version,
		// GroupID unknown from manifest/filename
	}, nil
}

// extractMavenCoordinatesFromPath extracts Maven coordinates from a Bazel-style path.
// Bazel stores Maven dependencies in paths like:
//
//	.../maven2/io/pyroscope/agent/2.1.2/processed_agent-2.1.2.jar
//	.../maven2/com/google/guava/guava/31.1-jre/guava-31.1-jre.jar
//
// The structure is: maven2/{groupId as path}/{artifactId}/{version}/{filename}
func extractMavenCoordinatesFromPath(jarPath string) *MavenCoordinates {
	// Normalize path separators
	jarPath = filepath.ToSlash(jarPath)

	// Find "maven2/" marker in path
	maven2Idx := strings.Index(jarPath, "maven2/")
	if maven2Idx == -1 {
		return &MavenCoordinates{}
	}

	// Get the path after "maven2/"
	pathAfterMaven2 := jarPath[maven2Idx+len("maven2/"):]

	// Split into segments: [groupId parts..., artifactId, version, filename]
	segments := strings.Split(pathAfterMaven2, "/")
	if len(segments) < 4 {
		return &MavenCoordinates{}
	}

	// Last segment is the filename
	// Second-to-last is version
	// Third-to-last is artifactId
	// Everything before that is groupId
	version := segments[len(segments)-2]
	artifactId := segments[len(segments)-3]
	groupIdParts := segments[:len(segments)-3]

	// Validate: version should look like a version
	if !looksLikeVersion(version) {
		return &MavenCoordinates{}
	}

	// Join groupId parts with dots
	groupId := strings.Join(groupIdParts, ".")

	return &MavenCoordinates{
		GroupID:    groupId,
		ArtifactID: artifactId,
		Version:    version,
	}
}

func (a *JARAnalyzer) extractArtifactInfoFromManifest(jarPath string) (artifactId, version string, err error) {
	manifest, err := a.ExtractManifest(jarPath)
	if err != nil {
		return "", "", fmt.Errorf("missing MANIFEST.MF: %w", err)
	}

	baseName := filepath.Base(jarPath)
	baseName = strings.TrimSuffix(baseName, ".jar")
	artifactId = baseName
	var versionFromFilename string
	parts := strings.Split(baseName, "-")
	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		if strings.ContainsAny(lastPart, "0123456789") {
			versionFromFilename = lastPart
			artifactId = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	version, ok := manifest["Implementation-Version"]
	if !ok || version == "" {
		if versionFromFilename != "" {
			version = versionFromFilename
		} else {
			return "", "", fmt.Errorf("missing Implementation-Version in manifest and could not extract from filename")
		}
	}

	return artifactId, version, nil
}

type JARExtractor struct{}

// ExtractThirdPartyJARs extracts 3rd party JARs from a JAR file.
// For Spring Boot fat JARs, it extracts nested JARs from BOOT-INF/lib/.
// For Bazel JARs, it finds dependencies in the .runfiles directory.
// For regular JARs, it returns the JAR itself for processing.
func (e *JARExtractor) ExtractThirdPartyJARs(jarPath string) ([]string, string, func() error, error) {
	mainJAR, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to open JAR: %w", err)
	}
	defer mainJAR.Close()

	// Check if this is a Spring Boot fat JAR
	if isSpringBootFatJAR(mainJAR.File) {
		return e.extractSpringBootJARs(mainJAR.File)
	}

	// Check for Bazel runfiles directory (e.g., ProjectRunner.runfiles for ProjectRunner.jar)
	bazelJARs := e.findBazelRunfileJARs(jarPath)
	if len(bazelJARs) > 0 {
		// Return both the main JAR and runfiles JARs
		allJARs := append([]string{jarPath}, bazelJARs...)
		return allJARs, "", func() error { return nil }, nil
	}

	// Regular JAR: return the JAR itself for processing
	return []string{jarPath}, "", func() error { return nil }, nil
}

// findBazelRunfileJARs finds 3rd party JARs in Bazel's runfiles directory.
// Bazel uses {name}.runfiles directory adjacent to {name}.jar for dependencies.
func (e *JARExtractor) findBazelRunfileJARs(jarPath string) []string {
	// Try both patterns:
	// 1. {name}.runfiles (for {name}.jar)
	// 2. {name}.jar.runfiles
	baseName := strings.TrimSuffix(jarPath, ".jar")
	runfilesDirs := []string{
		baseName + ".runfiles",
		jarPath + ".runfiles",
	}

	var jars []string
	for _, runfilesDir := range runfilesDirs {
		info, err := os.Stat(runfilesDir)
		if err != nil || !info.IsDir() {
			continue
		}

		// Walk the runfiles directory to find JAR files
		err = filepath.Walk(runfilesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".jar") {
				return nil
			}
			// Skip JDK/JRE JARs
			if strings.Contains(path, "local_jdk") || strings.Contains(path, "jre/lib") {
				return nil
			}
			// Skip the main JAR if it appears in runfiles
			if filepath.Base(path) == filepath.Base(jarPath) {
				return nil
			}
			jars = append(jars, path)
			return nil
		})
		if err != nil {
			continue
		}

		// If we found JARs, return them
		if len(jars) > 0 {
			return jars
		}
	}

	return nil
}

// isSpringBootFatJAR checks if a JAR file is a Spring Boot fat JAR by looking for
// the BOOT-INF directory structure and nested JARs in BOOT-INF/lib/.
func isSpringBootFatJAR(files []*zip.File) bool {
	jarPattern := regexp.MustCompile(`^BOOT-INF/lib/.*\.jar$`)
	hasBootInf := false
	hasNestedJARs := false

	for _, f := range files {
		if strings.HasPrefix(f.Name, "BOOT-INF/") {
			hasBootInf = true
		}
		if jarPattern.MatchString(f.Name) {
			hasNestedJARs = true
		}
		if hasBootInf && hasNestedJARs {
			return true
		}
	}

	return false
}

// extractSpringBootJARs extracts nested JAR files from BOOT-INF/lib/ in a Spring Boot fat JAR.
func (e *JARExtractor) extractSpringBootJARs(files []*zip.File) ([]string, string, func() error, error) {
	jarPattern := regexp.MustCompile(`^BOOT-INF/lib/.*\.jar$`)
	var jarFiles []string
	fileMap := make(map[string]*zip.File)

	// Collect all nested JAR files
	for _, f := range files {
		if jarPattern.MatchString(f.Name) {
			jarFiles = append(jarFiles, f.Name)
			fileMap[f.Name] = f
		}
	}

	if len(jarFiles) == 0 {
		// This shouldn't happen if isSpringBootFatJAR returned true,
		// but handle it gracefully
		return nil, "", nil, fmt.Errorf("spring boot fat JAR contains no nested JARs in BOOT-INF/lib/")
	}

	sort.Strings(jarFiles)

	// Create temporary directory for extracted JARs
	tmpDir, err := os.MkdirTemp("", "jar-mapper-*")
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cleanup := func() error {
		return os.RemoveAll(tmpDir)
	}

	// Extract each nested JAR to the temp directory
	var extractedJARs []string
	for _, jarFile := range jarFiles {
		f := fileMap[jarFile]
		extractedPath := filepath.Join(tmpDir, filepath.Base(jarFile))
		if err := extractFile(f, extractedPath); err != nil {
			fmt.Printf("Warning: failed to extract %s: %v\n", jarFile, err)
			continue
		}
		extractedJARs = append(extractedJARs, extractedPath)
	}

	return extractedJARs, tmpDir, cleanup, nil
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
