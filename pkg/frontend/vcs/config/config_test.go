package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePyroscopeConfig(t *testing.T) {
	t.Run("valid go config", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - language: go
      path:
        - prefix: GOROOT
      source:
        github:
          owner: golang
          repo: go
          ref: go1.24.8
          path: src
`
		config, err := ParsePyroscopeConfig([]byte(yaml))
		require.NoError(t, err)
		require.NotNil(t, config)

		require.Len(t, config.SourceCode.Mappings, 1)

		mapping := config.SourceCode.Mappings[0]
		assert.Equal(t, "go", mapping.Language)
		require.Len(t, mapping.Path, 1)
		assert.Equal(t, "GOROOT", mapping.Path[0].Prefix)
		require.NotNil(t, mapping.Source.GitHub)
		assert.Equal(t, "golang", mapping.Source.GitHub.Owner)
		assert.Equal(t, "go", mapping.Source.GitHub.Repo)
		assert.Equal(t, "go1.24.8", mapping.Source.GitHub.Ref)
		assert.Equal(t, "src", mapping.Source.GitHub.Path)
	})

	t.Run("valid java config with multiple mappings", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - language: java
      path:
        - prefix: org/example/rideshare
      source:
        local:
          path: src/main/java/org/example/rideshare
    - language: java
      path:
        - prefix: java
      source:
        github:
          owner: openjdk
          repo: jdk
          ref: jdk-17+0
          path: src/java.base/share/classes/java
    - language: java
      path:
        - prefix: org/springframework/http
      source:
        github:
          owner: spring-projects
          repo: spring-framework
          ref: v5.3.20
          path: spring-web/src/main/java/org/springframework/http
`
		config, err := ParsePyroscopeConfig([]byte(yaml))
		require.NoError(t, err)
		require.NotNil(t, config)

		require.Len(t, config.SourceCode.Mappings, 3)

		// Check first mapping (local)
		mapping1 := config.SourceCode.Mappings[0]
		assert.Equal(t, "java", mapping1.Language)
		require.Len(t, mapping1.Path, 1)
		assert.Equal(t, "org/example/rideshare", mapping1.Path[0].Prefix)
		require.NotNil(t, mapping1.Source.Local)
		assert.Equal(t, "src/main/java/org/example/rideshare", mapping1.Source.Local.Path)

		// Check second mapping (github)
		mapping2 := config.SourceCode.Mappings[1]
		assert.Equal(t, "java", mapping2.Language)
		require.Len(t, mapping2.Path, 1)
		assert.Equal(t, "java", mapping2.Path[0].Prefix)
		require.NotNil(t, mapping2.Source.GitHub)
		assert.Equal(t, "openjdk", mapping2.Source.GitHub.Owner)

		// Check third mapping (github)
		mapping3 := config.SourceCode.Mappings[2]
		assert.Equal(t, "java", mapping3.Language)
		require.Len(t, mapping3.Path, 1)
		assert.Equal(t, "org/springframework/http", mapping3.Path[0].Prefix)
		require.NotNil(t, mapping3.Source.GitHub)
		assert.Equal(t, "spring-projects", mapping3.Source.GitHub.Owner)
	})

	t.Run("invalid - missing language", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - path:
        - prefix: GOROOT
      source:
        github:
          owner: golang
          repo: go
          ref: go1.24.8
          path: src
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "language")
	})

	t.Run("invalid - missing source config", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - language: go
      path:
        - prefix: GOROOT
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no source type supplied")
	})

	t.Run("invalid - missing path and function_name", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - language: java
      source:
        local:
          path: src/main/java
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one path or a function_name match is required")
	})

	t.Run("invalid - unsupported language", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - language: python
      path:
        - prefix: GOROOT
      source:
        github:
          owner: golang
          repo: go
          ref: go1.24.8
          path: src
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})

	t.Run("invalid yaml syntax", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - language: go
      path:
        - prefix: GOROOT
      source:
        github:
          owner: golang
          repo: go
          ref: go1.24.8
          path: src
        github:
          owner: duplicate
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
	})

	t.Run("wrong version", func(t *testing.T) {
		yaml := `version: v1alpha1
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
	})
}

func TestFindMapping(t *testing.T) {
	config := &PyroscopeConfig{
		SourceCode: SourceCodeConfig{
			Mappings: []MappingConfig{
				{
					Language: "java",
					Path: []Match{
						{Prefix: "org/example/rideshare"},
					},
					Source: Source{
						Local: &LocalMappingConfig{
							Path: "src/main/java/org/example/rideshare",
						},
					},
				},
				{
					Language: "java",
					Path: []Match{
						{Prefix: "java"},
					},
					Source: Source{
						GitHub: &GitHubMappingConfig{
							Owner: "openjdk",
							Repo:  "jdk",
							Ref:   "jdk-17+0",
							Path:  "src/java.base/share/classes/java",
						},
					},
				},
				{
					Language: "java",
					Path: []Match{
						{Prefix: "org/springframework/http"},
					},
					Source: Source{
						GitHub: &GitHubMappingConfig{
							Owner: "spring-projects",
							Repo:  "spring-framework",
							Ref:   "v5.3.20",
							Path:  "spring-web/src/main/java/org/springframework/http",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		fileSpec       FileSpec
		expectedPrefix string
		expectedSource string // "local" or "github"
		shouldBeNil    bool
	}{
		{
			name:           "exact match for local",
			fileSpec:       FileSpec{Path: "org/example/rideshare"},
			expectedPrefix: "org/example/rideshare",
			expectedSource: "local",
		},
		{
			name:           "subdirectory of local",
			fileSpec:       FileSpec{Path: "org/example/rideshare/App.java"},
			expectedPrefix: "org/example/rideshare",
			expectedSource: "local",
		},
		{
			name:           "exact match for java",
			fileSpec:       FileSpec{Path: "java"},
			expectedPrefix: "java",
			expectedSource: "github",
		},
		{
			name:           "subdirectory of java",
			fileSpec:       FileSpec{Path: "java/util/ArrayList.java"},
			expectedPrefix: "java",
			expectedSource: "github",
		},
		{
			name:           "exact match for springframework",
			fileSpec:       FileSpec{Path: "org/springframework/http"},
			expectedPrefix: "org/springframework/http",
			expectedSource: "github",
		},
		{
			name:           "subdirectory of springframework",
			fileSpec:       FileSpec{Path: "org/springframework/http/HttpStatus.java"},
			expectedPrefix: "org/springframework/http",
			expectedSource: "github",
		},
		{
			name:           "longest prefix match",
			fileSpec:       FileSpec{Path: "org/springframework/http/converter/HttpMessageConverter.java"},
			expectedPrefix: "org/springframework/http",
			expectedSource: "github",
		},
		{
			name:        "no match",
			fileSpec:    FileSpec{Path: "com/google/common/collect/Lists.java"},
			shouldBeNil: true,
		},
		{
			name:        "partial prefix should not match",
			fileSpec:    FileSpec{Path: "organization/test/File.java"},
			shouldBeNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.FindMapping(tt.fileSpec)
			if tt.shouldBeNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				require.Len(t, result.Path, 1)
				assert.Equal(t, tt.expectedPrefix, result.Path[0].Prefix)
				switch tt.expectedSource {
				case "local":
					assert.NotNil(t, result.Source.Local)
				case "github":
					assert.NotNil(t, result.Source.GitHub)
				}
			}
		})
	}
}
