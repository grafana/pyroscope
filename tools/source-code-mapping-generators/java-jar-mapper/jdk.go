package main

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

// extractJDKVersionFromJAR extracts the JDK version from a JAR file by analyzing
// the class file major version numbers. Returns the major version (e.g., "8", "11", "17").
func extractJDKVersionFromJAR(jarPath string) (string, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return "", fmt.Errorf("failed to open JAR: %w", err)
	}
	defer reader.Close()

	var maxMajorVersion int
	for _, f := range reader.File {
		if !strings.HasSuffix(f.Name, ".class") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		// Read class file header to get major version
		// Class file format: magic (4 bytes) + minor_version (2 bytes) + major_version (2 bytes)
		header := make([]byte, 8)
		n, err := rc.Read(header)
		rc.Close()
		if err != nil && err != io.EOF {
			continue
		}
		if n < 8 {
			continue
		}

		// Check magic number (0xCAFEBABE)
		if binary.BigEndian.Uint32(header[0:4]) != 0xCAFEBABE {
			continue
		}

		// Extract major version (bytes 6-7)
		majorVersion := int(binary.BigEndian.Uint16(header[6:8]))
		if majorVersion > maxMajorVersion {
			maxMajorVersion = majorVersion
		}
	}

	if maxMajorVersion == 0 {
		return "", fmt.Errorf("no valid class files found in JAR")
	}

	// Map class file major version to JDK version
	// Java 8 = 52, Java 9 = 53, Java 10 = 54, Java 11 = 55, etc.
	jdkVersion := getJDKVersionInfo(maxMajorVersion).Version
	if jdkVersion == "" {
		return "", fmt.Errorf("unsupported class file version: %d", maxMajorVersion)
	}

	return jdkVersion, nil
}

type JDKVersionInfo struct {
	Version string
	Repo    string
	Path    string
}

// Class file major version to JDK version mapping
// Path defaults to "src/java.base/share/classes"
// Repo defaults to "jdk<version>"
var jdkVersionMap = map[int]*JDKVersionInfo{
	52: {Version: "8", Path: "jdk/src/share/classes"},
	53: {Version: "9", Path: "jdk/src/java.base/share/classes"},
	54: {Version: "10"},
	55: {Version: "11"},
	56: {Version: "12"},
	57: {Version: "13"},
	58: {Version: "14"},
	59: {Version: "15"},
	60: {Version: "16"},
	61: {Version: "17"},
	62: {Version: "18"},
	63: {Version: "19"},
	64: {Version: "20"},
	65: {Version: "21"},
	66: {Version: "22"},
	67: {Version: "23", Repo: "jdk23u"},
}

func getJDKVersionInfo(majorVersion int) *JDKVersionInfo {
	info := jdkVersionMap[majorVersion]
	if info == nil {
		return nil
	}

	// Apply defaults
	if info.Repo == "" {
		info.Repo = fmt.Sprintf("jdk%s", info.Version)
	}
	if info.Path == "" {
		info.Path = "src/java.base/share/classes"
	}

	return info
}

// jdkVersionToMajorVersion converts a JDK version string (e.g., "8", "11", "17") to class file major version.
func jdkVersionToMajorVersion(jdkVersion string) int {
	for majorVersion, info := range jdkVersionMap {
		if info.Version == jdkVersion {
			return majorVersion
		}
	}
	return 0
}

// generateJDKMappings generates mappings for JDK packages (java/, jdk/, javax/, sun/)
// jdkVersion should be a major version number like "8", "11", "17", "21", etc.
func generateJDKMappings(jdkVersion string) []config.MappingConfig {
	var mappings []config.MappingConfig

	majorVersion := jdkVersionToMajorVersion(jdkVersion)
	if majorVersion == 0 {
		return nil
	}
	version := getJDKVersionInfo(majorVersion)
	if version == nil {
		return nil
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
				Repo:  version.Repo,
				Ref:   "master",
				Path:  version.Path,
			},
		},
	}

	mappings = append(mappings, mapping)
	return mappings
}
