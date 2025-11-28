package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type Language string

const (
	PyroscopeConfigPath = ".pyroscope.yaml"

	LanguageUnknown = Language("")
	LanguageGo      = Language("go")
	LanguageJava    = Language("java")
)

type Version string

const (
	VersionUnknown = Version("")
	VersionV1      = Version("v1")
)

var validLanguages = []Language{
	LanguageGo,
	LanguageJava,
}

// PyroscopeConfig represents the structure of .pyroscope.yaml configuration file
type PyroscopeConfig struct {
	Version    Version
	SourceCode SourceCodeConfig `yaml:"source_code"`
}

// SourceCodeConfig contains source code mapping configuration
type SourceCodeConfig struct {
	Mappings []MappingConfig `yaml:"mappings"`
}

// MappingConfig represents a single source code path mapping
type MappingConfig struct {
	Path         []Match `yaml:"path"`
	FunctionName []Match `yaml:"function_name"`
	Language     string  `yaml:"language"`

	Source Source `yaml:"source"`
}

// Match represents how mappings a single source code path mapping
type Match struct {
	Prefix string `yaml:"prefix"`
}

// Source represents how mappings retrieve the source
type Source struct {
	Local  *LocalMappingConfig  `yaml:"local,omitempty"`
	GitHub *GitHubMappingConfig `yaml:"github,omitempty"`
}

// LocalMappingConfig contains configuration for local path mappings
type LocalMappingConfig struct {
	Path string `yaml:"path"`
}

// GitHubMappingConfig contains configuration for GitHub repository mappings
type GitHubMappingConfig struct {
	Owner string `yaml:"owner"`
	Repo  string `yaml:"repo"`
	Ref   string `yaml:"ref"`
	Path  string `yaml:"path"`
}

// ParsePyroscopeConfig parses a configuration from bytes
func ParsePyroscopeConfig(data []byte) (*PyroscopeConfig, error) {
	var config PyroscopeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse pyroscope config: %w", err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pyroscope config: %w", err)
	}

	return &config, nil
}

// Validate checks if the configuration is valid
func (c *PyroscopeConfig) Validate() error {
	if c.Version == VersionUnknown {
		c.Version = VersionV1
	}

	if c.Version != VersionV1 {
		return fmt.Errorf("invalid version '%s', supported versions are '%s'", c.Version, VersionV1)
	}

	var errs []error
	for i, mapping := range c.SourceCode.Mappings {
		if err := mapping.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("mapping[%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

// Validate checks if a mapping configuration is valid
func (m *MappingConfig) Validate() error {
	var errs []error

	if len(m.Path) == 0 && len(m.FunctionName) == 0 {
		errs = append(errs, fmt.Errorf("at least one path or a function_name match is required"))
	}

	if !slices.Contains(validLanguages, Language(m.Language)) {
		errs = append(errs, fmt.Errorf("language '%s' unsupported, valid languages are %v", m.Language, validLanguages))
	}

	if err := m.Source.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate checks if a source configuration is valid
func (m *Source) Validate() error {
	var (
		instances int
		errs      []error
	)

	if m.GitHub != nil {
		instances++
		if err := m.GitHub.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if m.Local != nil {
		instances++
		if err := m.Local.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	if instances == 0 {
		errs = append(errs, errors.New("no source type supplied, you need to supply exactly one source type"))
	} else if instances != 1 {
		errs = append(errs, errors.New("more than one source type supplied, you need to supply exactly one source type"))
	}

	return errors.Join(errs...)
}

func (m *GitHubMappingConfig) Validate() error {
	return nil
}

func (m *LocalMappingConfig) Validate() error {
	return nil
}

type FileSpec struct {
	Path         string
	FunctionName string
}

// FindMapping finds a mapping configuration that matches the given FileSpec
// Returns nil if no matching mapping is found
func (c *PyroscopeConfig) FindMapping(file FileSpec) *MappingConfig {
	if c == nil {
		return nil
	}

	// Find the longest matching prefix
	var bestMatch *MappingConfig
	var bestMatchLen = -1
	for _, m := range c.SourceCode.Mappings {
		if result := m.Match(file); result > bestMatchLen {
			bestMatch = &m
			bestMatchLen = result
		}
	}
	return bestMatch
}

// Returns -1 if no match, otherwise the number of characters that matched
func (m *MappingConfig) Match(file FileSpec) int {
	result := -1
	for _, fun := range m.FunctionName {
		if strings.HasPrefix(file.FunctionName, fun.Prefix) {
			if len(fun.Prefix) > result {
				result = len(fun.Prefix)
			}
		}
	}
	for _, path := range m.Path {
		if strings.HasPrefix(file.Path, path.Prefix) {
			if len(path.Prefix) > result {
				result = len(path.Prefix)
			}
		}
	}
	return result
}
