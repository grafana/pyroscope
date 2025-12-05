package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

//go:embed jar-mappings.json
var jarMappingsJSON []byte

// JarMapping represents a hardcoded mapping for a JAR file.
type JarMapping struct {
	Jar   string `json:"jar"`   // JAR name (artifactId) to match
	Owner string `json:"owner"` // GitHub owner
	Repo  string `json:"repo"`  // GitHub repository
	Path  string `json:"path"`  // Source path in repository
}

// JarMappingsConfig represents the JSON configuration file.
type JarMappingsConfig struct {
	Mappings []JarMapping `json:"mappings"`
}

// ConfigService handles loading and querying JAR mappings.
type ConfigService struct {
	mappings *JarMappingsConfig
}

func (s *ConfigService) LoadJarMappings() (*JarMappingsConfig, error) {
	if len(jarMappingsJSON) == 0 {
		s.mappings = nil
		return nil, nil
	}

	var config JarMappingsConfig
	if err := json.Unmarshal(jarMappingsJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JAR mappings JSON: %w", err)
	}

	s.mappings = &config
	return &config, nil
}

func (s *ConfigService) FindJarMapping(artifactId string) *JarMapping {
	if s.mappings == nil {
		return nil
	}

	for i := range s.mappings.Mappings {
		if s.mappings.Mappings[i].Jar == artifactId {
			return &s.mappings.Mappings[i]
		}
	}

	return nil
}

func GenerateOrMergeConfig(configPath string, mappings []config.MappingConfig, jdkMappings []config.MappingConfig) error {
	if len(jdkMappings) > 0 {
		mappings = append(mappings, jdkMappings...)
		fmt.Fprintf(os.Stderr, "Added %d JDK mapping(s)\n", len(jdkMappings))
	}

	var cfg config.PyroscopeConfig

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		existingCfg, err := config.ParsePyroscopeConfig(data)
		if err != nil {
			return fmt.Errorf("failed to parse existing config file: %w", err)
		}
		cfg = *existingCfg

		cfg.SourceCode.Mappings = MergeMappings(cfg.SourceCode.Mappings, mappings)
	} else {
		cfg = config.PyroscopeConfig{
			Version: config.VersionV1,
			SourceCode: config.SourceCodeConfig{
				Mappings: mappings,
			},
		}
	}

	SortMappings(cfg.SourceCode.Mappings)

	var output io.Writer = os.Stdout
	if configPath != "" {
		file, err := os.Create(configPath)
		if err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
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
