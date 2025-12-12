package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

// Processor processes individual JAR files to generate mapping configurations.
type Processor struct {
	jarAnalyzer      *JARAnalyzer
	githubResolver   *GitHubResolver
	configService    *ConfigService
	githubTagService *GitHubTagService
}

// NewProcessor creates a new Processor.
func NewProcessor(
	jarAnalyzer *JARAnalyzer,
	githubResolver *GitHubResolver,
	configService *ConfigService,
	githubTagService *GitHubTagService,
) *Processor {
	return &Processor{
		jarAnalyzer:      jarAnalyzer,
		githubResolver:   githubResolver,
		configService:    configService,
		githubTagService: githubTagService,
	}
}

func (p *Processor) ProcessJAR(jarPath string) (*config.MappingConfig, error) {
	prefixes, err := p.jarAnalyzer.ExtractClassPrefixes(jarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract class names: %w", err)
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no common prefixes found in class names")
	}

	coords, err := p.jarAnalyzer.ExtractMavenCoordinates(jarPath)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "Processing JAR: %s\n", filepath.Base(jarPath))
	fmt.Fprintf(os.Stderr, "  Coordinates: %s:%s:%s\n", coords.GroupID, coords.ArtifactID, coords.Version)

	owner, repo, err := p.githubResolver.ResolveRepo(coords)
	if err != nil {
		return nil, err
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("failed to resolve GitHub repo for %s", coords.ArtifactID)
	}

	ref := p.determineRef(owner, repo, coords.Version)

	sourcePath := determineSourcePath(coords.ArtifactID)

	// Check for hardcoded path override
	if p.configService != nil {
		if mapping := p.configService.FindJarMapping(coords.ArtifactID); mapping != nil && mapping.Path != "" {
			sourcePath = mapping.Path
		}
	}

	return p.buildMappingConfig(prefixes, owner, repo, ref, sourcePath), nil
}

func (p *Processor) ProcessJARWithCoords(jarPath string, coords *MavenCoordinates) (*config.MappingConfig, error) {
	prefixes, err := p.jarAnalyzer.ExtractClassPrefixes(jarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract class names: %w", err)
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no common prefixes found in class names")
	}

	fmt.Fprintf(os.Stderr, "Processing JAR: %s\n", filepath.Base(jarPath))
	fmt.Fprintf(os.Stderr, "  Coordinates: %s:%s:%s\n", coords.GroupID, coords.ArtifactID, coords.Version)

	owner, repo, err := p.githubResolver.ResolveRepo(coords)
	if err != nil {
		return nil, err
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("failed to resolve GitHub repo for %s", coords.ArtifactID)
	}

	ref := p.determineRef(owner, repo, coords.Version)
	sourcePath := determineSourcePath(coords.ArtifactID)

	if p.configService != nil {
		if mapping := p.configService.FindJarMapping(coords.ArtifactID); mapping != nil && mapping.Path != "" {
			sourcePath = mapping.Path
		}
	}

	return p.buildMappingConfig(prefixes, owner, repo, ref, sourcePath), nil
}

func (p *Processor) buildMappingConfig(prefixes []string, owner, repo, ref, sourcePath string) *config.MappingConfig {
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

	sortedPrefixes := make([]string, len(prefixes))
	copy(sortedPrefixes, prefixes)
	sort.Strings(sortedPrefixes)

	for i, prefix := range sortedPrefixes {
		mapping.FunctionName[i] = config.Match{Prefix: prefix}
	}

	return mapping
}

func (p *Processor) determineRef(owner, repo, version string) string {
	if p.githubTagService == nil {
		return version
	}

	ref, err := p.githubTagService.FindTagForVersion(owner, repo, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to query GitHub tags for %s/%s: %v\n", owner, repo, err)
		return version
	}

	fmt.Fprintf(os.Stderr, "  Found GitHub tag: %s (for version %s)\n", ref, version)
	return ref
}

// determineSourcePath determines the source path for Java code.
// Since path structures vary widely across projects, we use conservative defaults:
// - Simple artifactIds: src/main/java (standard Maven layout)
// - Multi-module projects: empty string (search from repo root)
// Known projects should use jar-mappings.json for correct paths.
func determineSourcePath(artifactId string) string {
	// Strip version suffixes (e.g., "_2.13", "_3", "_1.0")
	cleanArtifactId := stripVersionSuffix(artifactId)

	// For simple artifactIds without hyphens, use standard Maven src path
	if !strings.Contains(cleanArtifactId, "-") {
		return "src/main/java"
	}

	// For multi-module projects, we can't reliably determine the path
	// because naming conventions vary widely:
	//   - spark-core -> core/src/main/java (strips project prefix)
	//   - spring-web -> spring-web/src/main/java (keeps full name)
	//   - jackson-core -> src/main/java (single module at root)
	//
	// Return empty path to search from repo root
	return ""
}

// stripVersionSuffix removes version-like suffixes from artifactId.
// This handles Scala version suffixes (_2.13, _3) and similar patterns.
// Pattern: underscore followed by a version number (e.g., _2.13, _3, _1.0)
var versionSuffixPattern = regexp.MustCompile(`_\d+(\.\d+)*$`)

func stripVersionSuffix(artifactId string) string {
	return versionSuffixPattern.ReplaceAllString(artifactId, "")
}

// MappingService orchestrates the mapping generation process.
type MappingService struct {
	processor    *Processor
	jarExtractor *JARExtractor
}

func NewMappingService(processor *Processor, jarExtractor *JARExtractor) *MappingService {
	return &MappingService{
		processor:    processor,
		jarExtractor: jarExtractor,
	}
}

// ProcessJAR processes a JAR file and returns all mappings.
func (s *MappingService) ProcessJAR(jarPath string) ([]config.MappingConfig, error) {
	thirdPartyJARs, _, cleanup, err := s.jarExtractor.ExtractThirdPartyJARs(jarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract 3rd party JARs: %w", err)
	}
	defer cleanup() //nolint:errcheck

	fmt.Fprintf(os.Stderr, "Found %d JAR(s) to process\n", len(thirdPartyJARs))

	var mappings []config.MappingConfig
	successCount := 0
	failCount := 0

	for _, jarFile := range thirdPartyJARs {
		mapping, err := s.processor.ProcessJAR(jarFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ Skipping %s: %v\n", filepath.Base(jarFile), err)
			failCount++
			continue
		}

		if mapping != nil {
			mappings = append(mappings, *mapping)
			fmt.Fprintf(os.Stderr, "✓ Successfully mapped %s to %s/%s\n",
				filepath.Base(jarFile),
				mapping.Source.GitHub.Owner,
				mapping.Source.GitHub.Repo)
			successCount++
		} else {
			failCount++
		}
	}

	fmt.Fprintf(os.Stderr, "\nProcessed %d JARs: %d successful, %d failed\n",
		len(thirdPartyJARs), successCount, failCount)

	return mappings, nil
}

// MergeMappings merges new mappings into existing ones, avoiding duplicates.
func MergeMappings(existing, new []config.MappingConfig) []config.MappingConfig {
	result := make([]config.MappingConfig, 0, len(existing)+len(new))
	result = append(result, existing...)

	for _, newMapping := range new {
		isDuplicate := false
		for _, existingMapping := range existing {
			if mappingsEqual(newMapping, existingMapping) {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			result = append(result, newMapping)
		}
	}

	return result
}

// mappingsEqual checks if two mappings are effectively the same.
func mappingsEqual(m1, m2 config.MappingConfig) bool {
	if m1.Source.GitHub == nil || m2.Source.GitHub == nil {
		return false
	}

	gh1, gh2 := m1.Source.GitHub, m2.Source.GitHub
	if gh1.Owner != gh2.Owner || gh1.Repo != gh2.Repo || gh1.Ref != gh2.Ref {
		return false
	}

	for _, fn1 := range m1.FunctionName {
		for _, fn2 := range m2.FunctionName {
			if fn1.Prefix == fn2.Prefix {
				return true
			}
		}
	}

	return false
}

// SortMappings sorts mappings to ensure deterministic output order.
func SortMappings(mappings []config.MappingConfig) {
	sort.Slice(mappings, func(i, j int) bool {
		mi, mj := mappings[i], mappings[j]

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

		prefixI, prefixJ := "", ""
		if len(mi.FunctionName) > 0 {
			prefixI = mi.FunctionName[0].Prefix
		}
		if len(mj.FunctionName) > 0 {
			prefixJ = mj.FunctionName[0].Prefix
		}
		return prefixI < prefixJ
	})
}
