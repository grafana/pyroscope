package main

import (
	"fmt"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

// generateJDKMappings generates mappings for JDK packages (java/, jdk/, javax/, sun/)
// jdkVersion should be a major version number like "8", "11", "17", "21", etc.
func generateJDKMappings(jdkVersion string) []config.MappingConfig {
	var mappings []config.MappingConfig

	// Determine OpenJDK repository and path based on JDK version
	var repo, path, ref string
	// TODO - this ref is usually wrong. make this logic
	// more robust and add verification that the ref exists.
	ref = fmt.Sprintf("jdk-%v+28", jdkVersion)

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
