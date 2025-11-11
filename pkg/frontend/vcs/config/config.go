package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// PyroscopeConfig represents the structure of .pyroscope.yaml configuration file
type PyroscopeConfig struct {
	SourceCode SourceCodeConfig `yaml:"source_code"`
}

// SourceCodeConfig contains source code mapping configuration
type SourceCodeConfig struct {
	Language string            `yaml:"language"`
	Mappings []MappingConfig   `yaml:"mappings"`
}

// MappingConfig represents a single source code path mapping
type MappingConfig struct {
	Path   string              `yaml:"path"`
	Type   string              `yaml:"type"`
	Local  *LocalMappingConfig `yaml:"local,omitempty"`
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

// ParsePyroscopeConfig parses a .pyroscope.yaml configuration from bytes
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
	if c.SourceCode.Language == "" {
		return fmt.Errorf("source_code.language is required")
	}

	for i, mapping := range c.SourceCode.Mappings {
		if err := mapping.Validate(); err != nil {
			return fmt.Errorf("mapping[%d]: %w", i, err)
		}
	}

	return nil
}

// Validate checks if a mapping configuration is valid
func (m *MappingConfig) Validate() error {
	if m.Path == "" {
		return fmt.Errorf("path is required")
	}

	if m.Type == "" {
		return fmt.Errorf("type is required")
	}

	switch m.Type {
	case "local":
		if m.Local == nil {
			return fmt.Errorf("local configuration is required when type is 'local'")
		}
		if m.Local.Path == "" {
			return fmt.Errorf("local.path is required")
		}
	case "github":
		if m.GitHub == nil {
			return fmt.Errorf("github configuration is required when type is 'github'")
		}
		if m.GitHub.Owner == "" {
			return fmt.Errorf("github.owner is required")
		}
		if m.GitHub.Repo == "" {
			return fmt.Errorf("github.repo is required")
		}
		if m.GitHub.Ref == "" {
			return fmt.Errorf("github.ref is required")
		}
		if m.GitHub.Path == "" {
			return fmt.Errorf("github.path is required")
		}
	default:
		return fmt.Errorf("unsupported type '%s', must be 'local' or 'github'", m.Type)
	}

	return nil
}

// FindMapping finds a mapping configuration that matches the given path
// Returns nil if no matching mapping is found
func (c *PyroscopeConfig) FindMapping(path string) *MappingConfig {
	// Find the longest matching prefix
	var bestMatch *MappingConfig
	var bestMatchLen int

	for i := range c.SourceCode.Mappings {
		mapping := &c.SourceCode.Mappings[i]
		if len(mapping.Path) > bestMatchLen && hasPrefix(path, mapping.Path) {
			bestMatch = mapping
			bestMatchLen = len(mapping.Path)
		}
	}

	return bestMatch
}

// hasPrefix checks if path starts with prefix, considering path separators
func hasPrefix(path, prefix string) bool {
	// Empty prefix doesn't match anything
	if prefix == "" {
		return false
	}

	if len(path) < len(prefix) {
		return false
	}

	if path[:len(prefix)] != prefix {
		return false
	}

	// Exact match
	if len(path) == len(prefix) {
		return true
	}

	// Check that the next character is a path separator
	nextChar := path[len(prefix)]
	return nextChar == '/' || nextChar == '\\'
}
