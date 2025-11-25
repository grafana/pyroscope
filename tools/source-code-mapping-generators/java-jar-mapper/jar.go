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

	prefixCount := make(map[string]int)
	for _, pkg := range packages {
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

func (a *JARAnalyzer) ExtractArtifactInfo(jarPath string) (artifactId, version string, err error) {
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

// ExtractThirdPartyJARs extracts 3rd party JARs from a Spring Boot JAR file.
func (e *JARExtractor) ExtractThirdPartyJARs(jarPath string) ([]string, string, func() error, error) {
	cmd := exec.Command("jar", "-tf", jarPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to list JAR contents: %w", err)
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

	sort.Strings(jarFiles)

	tmpDir, err := os.MkdirTemp("", "jar-mapper-*")
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cleanup := func() error {
		return os.RemoveAll(tmpDir)
	}

	var extractedJARs []string
	mainJAR, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, tmpDir, cleanup, fmt.Errorf("failed to open JAR: %w", err)
	}
	defer mainJAR.Close()

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
