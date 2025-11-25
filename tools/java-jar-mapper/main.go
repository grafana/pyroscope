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
	"sort"
	"strings"
	"time"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
	"gopkg.in/yaml.v3"
)

// JarMapping represents a hardcoded mapping for a JAR file
type JarMapping struct {
	Jar   string `json:"jar"`   // JAR name (artifactId) to match
	Owner string `json:"owner"` // GitHub owner
	Repo  string `json:"repo"`  // GitHub repository
	Path  string `json:"path"`  // Source path in repository
	// Ref is always inferred from the JAR file's version
}

// JarMappingsConfig represents the JSON configuration file
type JarMappingsConfig struct {
	Mappings []JarMapping `json:"mappings"`
}

var jarMappings *JarMappingsConfig

func main() {
	var (
		jarPath    = flag.String("jar", "", "Path to the Java JAR file to analyze")
		output     = flag.String("output", "", "Output file path (default: stdout)")
		jdkVersion = flag.String("jdk-version", "", "JDK version for JDK function mappings (e.g., '8', '11', '17', '21'). If not specified, JDK mappings will not be generated.")
		help       = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help || *jarPath == "" {
		fmt.Println("Java JAR Source Code Mapper")
		fmt.Println()
		fmt.Println("Generates .pyroscope.yaml source_code mappings for 3rd party libraries")
		fmt.Println("found in a Java JAR file.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s -jar <jar-file> [-output <output-file>] [-jdk-version <version>]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		return
	}

	// Load hardcoded JAR mappings
	if err := loadJarMappings(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load JAR mappings: %v\n", err)
		// Continue anyway, mappings are optional
	}

	if err := processJAR(*jarPath, *output, *jdkVersion); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// loadJarMappings loads the hardcoded JAR mappings from JSON file
func loadJarMappings() error {
	// Try to find the mappings file relative to the executable
	exePath, err := os.Executable()
	if err != nil {
		// Fallback to current directory
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)

	// Try multiple locations
	possiblePaths := []string{
		filepath.Join(exeDir, "jar-mappings.json"),
		filepath.Join(exeDir, "tools", "java-jar-mapper", "jar-mappings.json"),
		"tools/java-jar-mapper/jar-mappings.json",
		"jar-mappings.json",
	}

	var data []byte
	var found bool
	for _, path := range possiblePaths {
		if data, err = os.ReadFile(path); err == nil {
			found = true
			break
		}
	}

	if !found {
		// Mappings file is optional, return nil if not found
		return nil
	}

	var config JarMappingsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse JAR mappings JSON: %w", err)
	}

	jarMappings = &config
	return nil
}

// findJarMapping looks up a hardcoded mapping for the given artifactId
func findJarMapping(artifactId string) *JarMapping {
	if jarMappings == nil {
		return nil
	}

	for i := range jarMappings.Mappings {
		if jarMappings.Mappings[i].Jar == artifactId {
			return &jarMappings.Mappings[i]
		}
	}

	return nil
}

func processJAR(jarPath, outputPath string, jdkVersion string) error {
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

	// Auto-detect JDK version if not specified
	if jdkVersion == "" {
		detectedVersion := detectJDKVersion(jarPath)
		if detectedVersion != "" {
			jdkVersion = detectedVersion
			fmt.Printf("Auto-detected JDK version: %s\n", jdkVersion)
		}
	}

	// Add JDK mappings if JDK version is available (either specified or auto-detected)
	if jdkVersion != "" {
		jdkMappings := generateJDKMappings(jdkVersion)
		if len(jdkMappings) > 0 {
			mappings = append(mappings, jdkMappings...)
			fmt.Printf("Added %d JDK mapping(s) for JDK %s\n", len(jdkMappings), jdkVersion)
		}
	} else {
		fmt.Printf("JDK version not specified and could not be auto-detected. Skipping JDK mappings.\n")
	}

	// Sort mappings to ensure deterministic output order
	// Sort by: owner, repo, ref, then by first function name prefix
	sort.Slice(mappings, func(i, j int) bool {
		mi, mj := mappings[i], mappings[j]
		
		// Compare GitHub owner
		ownerI, ownerJ := "", ""
		if mi.Source.GitHub != nil {
			ownerI = mi.Source.GitHub.Owner
		}
		if mj.Source.GitHub != nil {
			ownerJ = mj.Source.GitHub.Owner
		}
		if ownerI != ownerJ {
			return ownerI < ownerJ
		}
		
		// Compare repo
		repoI, repoJ := "", ""
		if mi.Source.GitHub != nil {
			repoI = mi.Source.GitHub.Repo
		}
		if mj.Source.GitHub != nil {
			repoJ = mj.Source.GitHub.Repo
		}
		if repoI != repoJ {
			return repoI < repoJ
		}
		
		// Compare ref
		refI, refJ := "", ""
		if mi.Source.GitHub != nil {
			refI = mi.Source.GitHub.Ref
		}
		if mj.Source.GitHub != nil {
			refJ = mj.Source.GitHub.Ref
		}
		if refI != refJ {
			return refI < refJ
		}
		
		// Compare first function name prefix
		prefixI, prefixJ := "", ""
		if len(mi.FunctionName) > 0 {
			prefixI = mi.FunctionName[0].Prefix
		}
		if len(mj.FunctionName) > 0 {
			prefixJ = mj.FunctionName[0].Prefix
		}
		return prefixI < prefixJ
	})

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

	// Sort JAR files to ensure deterministic processing order
	sort.Strings(jarFiles)

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

	// Create a map of file names to zip.File for deterministic lookups
	fileMap := make(map[string]*zip.File)
	for i := range mainJAR.File {
		fileMap[mainJAR.File[i].Name] = mainJAR.File[i]
	}

	for _, jarFile := range jarFiles {
		if f, ok := fileMap[jarFile]; ok {
			extractedPath := filepath.Join(tmpDir, filepath.Base(jarFile))
			if err := extractFile(f, extractedPath); err != nil {
				fmt.Printf("Warning: failed to extract %s: %v\n", jarFile, err)
				continue
			}
			extractedJARs = append(extractedJARs, extractedPath)
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

// determineRef determines the appropriate ref (tag) format for a GitHub repository.
// If the version already has "v" prefix, it's returned as-is.
// Otherwise, "v" prefix is added by default (most repos use this format).
// The VCS layer will automatically try both "v" and non-"v" prefix versions when fetching files/commits,
// so this is just for the YAML output to use the most common format.
func determineRef(version, owner, repo string) string {
	// If version already has "v" prefix, return as-is
	if strings.HasPrefix(version, "v") {
		return version
	}

	// Default: add "v" prefix (most repos use this format)
	// The VCS layer will try both "v" and non-"v" versions, so repos that don't use "v" will still work
	return "v" + version
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

	// Extract artifactId and version from filename first
	baseName := filepath.Base(jarPath)
	baseName = strings.TrimSuffix(baseName, ".jar")
	// Try to remove version suffix (pattern: -X.Y.Z or -X.Y)
	// This is a simple heuristic: remove last hyphen-separated parts that look like versions
	artifactId := baseName
	var versionFromFilename string
	parts := strings.Split(baseName, "-")
	if len(parts) > 1 {
		// Check if last part looks like a version (contains digits)
		lastPart := parts[len(parts)-1]
		if strings.ContainsAny(lastPart, "0123456789") {
			// Extract version from filename
			versionFromFilename = lastPart
			// Remove version parts to get artifactId
			artifactId = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	// Try to get version from manifest, fallback to filename
	version, ok := manifest["Implementation-Version"]
	if !ok || version == "" {
		if versionFromFilename != "" {
			version = versionFromFilename
		} else {
			return nil, fmt.Errorf("missing Implementation-Version in manifest and could not extract from filename")
		}
	}

	fmt.Fprintf(os.Stderr, "Processing JAR: %s\n", filepath.Base(jarPath))
	fmt.Fprintf(os.Stderr, "  Extracted artifactId: %s\n", artifactId)
	fmt.Fprintf(os.Stderr, "  Extracted version: %s\n", version)

	// Check for hardcoded mapping first
	if mapping := findJarMapping(artifactId); mapping != nil {
		fmt.Fprintf(os.Stderr, "  Found hardcoded mapping: %s/%s\n", mapping.Owner, mapping.Repo)
		// Auto-detect ref prefix based on repository and version
		ref := determineRef(version, mapping.Owner, mapping.Repo)

		mappingConfig := &config.MappingConfig{
			FunctionName: make([]config.Match, len(prefixes)),
			Language:     "java",
			Source: config.Source{
				GitHub: &config.GitHubMappingConfig{
					Owner: mapping.Owner,
					Repo:  mapping.Repo,
					Ref:   ref,
					Path:  mapping.Path,
				},
			},
		}

		// Sort prefixes to ensure deterministic output
		sortedPrefixes := make([]string, len(prefixes))
		copy(sortedPrefixes, prefixes)
		sort.Strings(sortedPrefixes)

		for i, prefix := range sortedPrefixes {
			mappingConfig.FunctionName[i] = config.Match{Prefix: prefix}
		}

		return mappingConfig, nil
	}

	var owner, repo string
	var pom []byte
	var groupId string

	// Query Maven Central
	fmt.Fprintf(os.Stderr, "  Attempting to fetch POM from Maven Central...\n")
	pom, groupId, err = fetchPOMFromMavenCentral(artifactId, version)
	if err != nil {
		return nil, err
	}

	// If groupId is still empty, try to extract from POM
	if groupId == "" {
		groupId, _ = extractGroupIdFromPOM(pom)
	}

	fmt.Fprintf(os.Stderr, "  Found groupId: %s\n", groupId)

	// Parse POM for SCM info and URL
	var pomStruct POM
	if err := xml.Unmarshal(pom, &pomStruct); err != nil {
		return nil, fmt.Errorf("failed to parse POM: %w", err)
	}

	// Try SCM first
	if pomStruct.SCM.URL != "" || pomStruct.SCM.Connection != "" {
		fmt.Fprintf(os.Stderr, "  POM has SCM info: URL=%s, Connection=%s\n", pomStruct.SCM.URL, pomStruct.SCM.Connection)
		scmInfo := &pomStruct.SCM
		if scmInfo.URL == "" {
			scmInfo.URL = scmInfo.Connection
		}
		var err error
		owner, repo, err = extractGitHubRepo(scmInfo)
		if err == nil {
			fmt.Fprintf(os.Stderr, "  Successfully extracted GitHub repo from SCM: %s/%s\n", owner, repo)
			// Successfully extracted from SCM
		} else {
			fmt.Fprintf(os.Stderr, "  SCM URL is not GitHub, trying POM URL field...\n")
			// SCM exists but not GitHub, try POM URL field
			if pomStruct.URL != "" {
				fmt.Fprintf(os.Stderr, "  POM URL field: %s\n", pomStruct.URL)
				owner, repo, err = extractGitHubRepoFromURL(pomStruct.URL)
			}
			// If still failed, try deps.dev
			if err != nil && groupId != "" {
				fmt.Fprintf(os.Stderr, "  Falling back to deps.dev...\n")
				owner, repo, err = queryDepsDevForGitHubRepo(groupId, artifactId, version)
				if err != nil {
					return nil, fmt.Errorf("invalid GitHub URL format (%s) and deps.dev query failed: %w", scmInfo.URL, err)
				}
			} else if err != nil {
				if groupId == "" {
					return nil, fmt.Errorf("invalid GitHub URL format (%s) and could not extract groupId for deps.dev query", scmInfo.URL)
				}
				return nil, fmt.Errorf("invalid GitHub URL format (%s) and deps.dev query failed", scmInfo.URL)
			}
		}
	} else {
		// No SCM, try parent POM first
		fmt.Fprintf(os.Stderr, "  POM has no SCM info, trying parent POM...\n")
		var err error
		if pomStruct.Parent.GroupID != "" && pomStruct.Parent.ArtifactID != "" && pomStruct.Parent.Version != "" {
			fmt.Fprintf(os.Stderr, "  Parent POM: %s:%s:%s\n", pomStruct.Parent.GroupID, pomStruct.Parent.ArtifactID, pomStruct.Parent.Version)
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
			fmt.Fprintf(os.Stderr, "  Trying POM URL field: %s\n", pomStruct.URL)
			owner, repo, err = extractGitHubRepoFromURL(pomStruct.URL)
		}
		// If still failed, try deps.dev
		if err != nil && groupId != "" {
			fmt.Fprintf(os.Stderr, "  Falling back to deps.dev...\n")
			owner, repo, err = queryDepsDevForGitHubRepo(groupId, artifactId, version)
			if err != nil {
				return nil, fmt.Errorf("POM missing SCM information and deps.dev query failed: %w", err)
			}
		} else if err != nil {
			if groupId == "" {
				return nil, fmt.Errorf("POM missing SCM information and could not extract groupId for deps.dev query")
			}
			return nil, fmt.Errorf("POM missing SCM information and deps.dev query failed: %w", err)
		}
	}

	// Validate that we have valid owner and repo
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("failed to extract valid GitHub owner/repo (owner: %q, repo: %q)", owner, repo)
	}

	// Determine ref (use version from manifest)
	ref := determineRef(version, owner, repo)

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

	// Sort prefixes to ensure deterministic output
	sortedPrefixes := make([]string, len(prefixes))
	copy(sortedPrefixes, prefixes)
	sort.Strings(sortedPrefixes)

	for i, prefix := range sortedPrefixes {
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
	packageSet := make(map[string]bool) // Use a set to deduplicate

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

	// Convert set to sorted slice for deterministic processing
	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

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
	// Sort keys first to ensure deterministic iteration
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

	// Sort deterministically: first by length (longest first), then alphabetically
	sort.Slice(commonPrefixes, func(i, j int) bool {
		lenI, lenJ := len(commonPrefixes[i]), len(commonPrefixes[j])
		if lenI != lenJ {
			return lenI > lenJ // Longer prefixes first
		}
		return commonPrefixes[i] < commonPrefixes[j] // Alphabetical for same length
	})

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
		fmt.Fprintf(os.Stderr, "    Trying POM URL: %s\n", url)

		resp, err := client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Fprintf(os.Stderr, "    ✓ Found POM at: %s\n", url)
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
	fmt.Fprintf(os.Stderr, "    All direct POM URLs failed, trying Maven Central search API...\n")
	pom, groupId, err := searchMavenCentralForPOMAny(artifactId, version)
	if err == nil {
		fmt.Fprintf(os.Stderr, "    ✓ Found POM via search API, groupId: %s\n", groupId)
		return pom, groupId, nil
	}

	return nil, "", fmt.Errorf("failed to fetch POM from %s (HTTP %v)", lastURL, lastErr)
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

	// Filter to only docs that match the exact artifactId we're looking for
	// and sort by groupId to ensure deterministic processing
	var matchingDocs []struct {
		GroupID    string
		ArtifactID string
		Version    string
	}
	for _, doc := range searchResp.Response.Docs {
		if doc.ArtifactID == artifactId {
			matchingDocs = append(matchingDocs, struct {
				GroupID    string
				ArtifactID string
				Version    string
			}{
				GroupID:    doc.GroupID,
				ArtifactID: doc.ArtifactID,
				Version:    doc.Version,
			})
		}
	}

	// Sort by groupId to ensure deterministic processing
	sort.Slice(matchingDocs, func(i, j int) bool {
		return matchingDocs[i].GroupID < matchingDocs[j].GroupID
	})

	// Try each matching result - fetch POM with exact version
	for _, doc := range matchingDocs {
		// Fetch the POM for this groupId/artifactId/version
		groupIdPath := strings.ReplaceAll(doc.GroupID, ".", "/")
		pomURL := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
			groupIdPath, artifactId, version, artifactId, version)

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

		// Return the first valid POM found (after sorting, this will be deterministic)
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

	fmt.Fprintf(os.Stderr, "Querying deps.dev for groupId=%s, artifactId=%s, version=%s\n", groupId, artifactId, version)
	fmt.Fprintf(os.Stderr, "  API endpoints:\n")
	for _, endpoint := range endpoints {
		fmt.Fprintf(os.Stderr, "    - %s\n", endpoint)
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
	fmt.Fprintf(os.Stderr, "  HTML fallback: %s\n", htmlURL)
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

// generateJDKMappings generates mappings for JDK packages (java/, jdk/, javax/, sun/)
// jdkVersion should be a major version number like "8", "11", "17", "21", etc.
func generateJDKMappings(jdkVersion string) []config.MappingConfig {
	var mappings []config.MappingConfig

	// Determine OpenJDK repository and path based on JDK version
	var repo, path, ref string
	versionNum := 0
	fmt.Sscanf(jdkVersion, "%d", &versionNum)

	if versionNum == 8 {
		// Java 8 uses jdk8u repository with different path structure
		repo = "jdk8u"
		path = "jdk/src/share/classes"
		// Use a common Java 8 tag (jdk8u402-b06 is a recent LTS update)
		ref = "jdk8u402-b06"
	} else if versionNum >= 9 {
		// Java 9+ uses main jdk repository
		repo = "jdk"
		path = "src/java.base/share/classes"
		// Use tag format jdk-XX+XX (e.g., jdk-11+28, jdk-17+35)
		// For simplicity, use a common tag for each major version
		// Users can override this if needed
		ref = fmt.Sprintf("jdk-%d+28", versionNum)
	} else {
		// Unsupported version, return empty
		fmt.Fprintf(os.Stderr, "Warning: Unsupported JDK version %s, skipping JDK mappings\n", jdkVersion)
		return mappings
	}

	// JDK package prefixes to map
	// These cover the most common JDK packages that appear in profiles
	jdkPrefixes := []string{
		"java/lang",
		"java/util",
		"java/io",
		"java/net",
		"java/time",
		"java/reflect",
		"java/security",
		"java/math",
		"java/text",
		"java/nio",
		"java/concurrent",
		"java/beans",
		"java/awt",
		"java/applet",
		"javax/annotation",
		"javax/crypto",
		"javax/net",
		"javax/security",
		"javax/sql",
		"javax/xml",
		"jdk/internal",
		"jdk/nashorn",
		"sun/misc",
		"sun/nio",
		"sun/reflect",
		"sun/security",
		"sun/util",
	}

	// Create a single mapping with all JDK prefixes
	// This is more efficient than creating separate mappings for each prefix
	matchConfigs := make([]config.Match, len(jdkPrefixes))
	for i, prefix := range jdkPrefixes {
		matchConfigs[i] = config.Match{Prefix: prefix}
	}

	mapping := config.MappingConfig{
		FunctionName: matchConfigs,
		Language:     "java",
		Source: config.Source{
			GitHub: &config.GitHubMappingConfig{
				Owner: "openjdk",
				Repo:  repo,
				Ref:   ref,
				Path:  path,
			},
		},
	}

	mappings = append(mappings, mapping)
	return mappings
}

// detectJDKVersion attempts to detect the JDK version from the JAR manifest
// Returns empty string if detection fails
func detectJDKVersion(jarPath string) string {
	manifest, err := extractManifest(jarPath)
	if err != nil {
		// Manifest extraction failed, try other methods
		return detectJDKVersionFromSystem()
	}

	// Try X-Compile-Target-JDK first (most reliable)
	if version := manifest["X-Compile-Target-JDK"]; version != "" {
		// Extract major version number (e.g., "8" from "1.8" or "8")
		majorVersion := extractMajorJDKVersion(version)
		if majorVersion != "" {
			return majorVersion
		}
	}

	// Try X-Compile-Source-JDK
	if version := manifest["X-Compile-Source-JDK"]; version != "" {
		majorVersion := extractMajorJDKVersion(version)
		if majorVersion != "" {
			return majorVersion
		}
	}

	// Try Build-Jdk (Maven format)
	if version := manifest["Build-Jdk"]; version != "" {
		majorVersion := extractMajorJDKVersion(version)
		if majorVersion != "" {
			return majorVersion
		}
	}

	// Try Build-Jdk-Spec (Maven format)
	if version := manifest["Build-Jdk-Spec"]; version != "" {
		majorVersion := extractMajorJDKVersion(version)
		if majorVersion != "" {
			return majorVersion
		}
	}

	// Fallback to system detection
	return detectJDKVersionFromSystem()
}

// extractMajorJDKVersion extracts the major version number from various JDK version formats
// Handles formats like: "1.8", "8", "11", "17.0.1", "21.0.2", etc.
func extractMajorJDKVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}

	// Handle "1.8" or "1.7" format (Java 8 and earlier)
	if strings.HasPrefix(version, "1.") {
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			return parts[1] // Return "8" from "1.8"
		}
	}

	// Handle modern format: "8", "11", "17", "21", etc.
	// Extract first number
	parts := strings.Split(version, ".")
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

// detectJDKVersionFromSystem attempts to detect JDK version from system
// This is a fallback when JAR manifest doesn't contain version info
func detectJDKVersionFromSystem() string {
	// Try to get version from java.version system property via exec
	// This is a best-effort approach
	cmd := exec.Command("java", "-XshowSettings:properties", "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	// Parse output for java.version
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "java.version") {
			// Extract version from line like "    java.version = 11.0.19"
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				version := strings.TrimSpace(parts[1])
				majorVersion := extractMajorJDKVersion(version)
				if majorVersion != "" {
					return majorVersion
				}
			}
		}
	}

	return ""
}
