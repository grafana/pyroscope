package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

// Processor processes individual JAR files to generate mapping configurations.
type Processor struct {
	jarAnalyzer    *JARAnalyzer
	mavenService   *MavenService
	githubResolver *GitHubResolver
	pomParser      *POMParser
	configService  *ConfigService
}

// NewProcessor creates a new Processor.
func NewProcessor(
	jarAnalyzer *JARAnalyzer,
	mavenService *MavenService,
	githubResolver *GitHubResolver,
	pomParser *POMParser,
	configService *ConfigService,
) *Processor {
	return &Processor{
		jarAnalyzer:    jarAnalyzer,
		mavenService:   mavenService,
		githubResolver: githubResolver,
		pomParser:      pomParser,
		configService:  configService,
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

	artifactId, version, err := p.jarAnalyzer.ExtractArtifactInfo(jarPath)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "Processing JAR: %s\n", filepath.Base(jarPath))
	fmt.Fprintf(os.Stderr, "  Extracted artifactId: %s\n", artifactId)
	fmt.Fprintf(os.Stderr, "  Extracted version: %s\n", version)

	jarMapping := p.configService.FindJarMapping(artifactId)
	if jarMapping != nil {
		return p.processJARWithHardcodedMapping(jarMapping, prefixes, version), nil
	}

	return p.processJARWithPOMLookup(artifactId, version, prefixes)
}

func (p *Processor) processJARWithHardcodedMapping(jarMapping *JarMapping, prefixes []string, version string) *config.MappingConfig {
	fmt.Fprintf(os.Stderr, "  Found hardcoded mapping: %s/%s\n", jarMapping.Owner, jarMapping.Repo)
	ref := determineRef(version, jarMapping.Owner, jarMapping.Repo)

	mappingConfig := &config.MappingConfig{
		FunctionName: make([]config.Match, len(prefixes)),
		Language:     "java",
		Source: config.Source{
			GitHub: &config.GitHubMappingConfig{
				Owner: jarMapping.Owner,
				Repo:  jarMapping.Repo,
				Ref:   ref,
				Path:  jarMapping.Path,
			},
		},
	}

	sortedPrefixes := make([]string, len(prefixes))
	copy(sortedPrefixes, prefixes)
	sort.Strings(sortedPrefixes)

	for i, prefix := range sortedPrefixes {
		mappingConfig.FunctionName[i] = config.Match{Prefix: prefix}
	}

	return mappingConfig
}

func (p *Processor) processJARWithPOMLookup(artifactId, version string, prefixes []string) (*config.MappingConfig, error) {
	fmt.Fprintf(os.Stderr, "  Attempting to fetch POM from Maven Central...\n")
	pomData, groupId, err := p.mavenService.FetchPOM(artifactId, version)
	if err != nil {
		return nil, err
	}

	if groupId == "" {
		groupId, _ = p.pomParser.ExtractGroupID(pomData)
	}

	fmt.Fprintf(os.Stderr, "  Found groupId: %s\n", groupId)

	pomStruct, err := p.pomParser.Parse(pomData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse POM: %w", err)
	}

	jarMapping := p.configService.FindJarMapping(artifactId)
	owner, repo, err := p.githubResolver.ResolveRepo(jarMapping, pomStruct, groupId, artifactId, version)
	if err != nil {
		return nil, err
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("failed to extract valid GitHub owner/repo (owner: %q, repo: %q)", owner, repo)
	}

	ref := determineRef(version, owner, repo)
	sourcePath := DetermineSourcePath(artifactId, pomStruct)

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

func determineRef(version, owner, repo string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
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
	defer cleanup()

	fmt.Printf("Found %d 3rd party JARs\n", len(thirdPartyJARs))

	var mappings []config.MappingConfig
	successCount := 0
	failCount := 0

	for _, jarFile := range thirdPartyJARs {
		fmt.Printf("Processing JAR: %s\n", filepath.Base(jarFile))

		mapping, err := s.processor.ProcessJAR(jarFile)
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
