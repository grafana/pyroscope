package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePyroscopeConfig(t *testing.T) {
	t.Run("valid go config", func(t *testing.T) {
		yaml := `source_code:
  language: go
  mappings:
    - path: GOROOT
      type: github
      github:
        owner: golang
        repo: go
        ref: go1.24.8
        path: src
`
		config, err := ParsePyroscopeConfig([]byte(yaml))
		require.NoError(t, err)
		require.NotNil(t, config)

		assert.Equal(t, "go", config.SourceCode.Language)
		require.Len(t, config.SourceCode.Mappings, 1)

		mapping := config.SourceCode.Mappings[0]
		assert.Equal(t, "GOROOT", mapping.Path)
		assert.Equal(t, "github", mapping.Type)
		require.NotNil(t, mapping.GitHub)
		assert.Equal(t, "golang", mapping.GitHub.Owner)
		assert.Equal(t, "go", mapping.GitHub.Repo)
		assert.Equal(t, "go1.24.8", mapping.GitHub.Ref)
		assert.Equal(t, "src", mapping.GitHub.Path)
	})

	t.Run("valid java config with multiple mappings", func(t *testing.T) {
		yaml := `source_code:
  language: java
  mappings:
    - path: org/example/rideshare
      type: local
      local:
        path: src/main/java/org/example/rideshare
    - path: java
      type: github
      github:
        owner: openjdk
        repo: jdk
        ref: jdk-17+0
        path: src/java.base/share/classes/java
    - path: org/springframework/http
      type: github
      github:
        owner: spring-projects
        repo: spring-framework
        ref: v5.3.20
        path: spring-web/src/main/java/org/springframework/http
`
		config, err := ParsePyroscopeConfig([]byte(yaml))
		require.NoError(t, err)
		require.NotNil(t, config)

		assert.Equal(t, "java", config.SourceCode.Language)
		require.Len(t, config.SourceCode.Mappings, 3)

		// Check first mapping (local)
		mapping1 := config.SourceCode.Mappings[0]
		assert.Equal(t, "org/example/rideshare", mapping1.Path)
		assert.Equal(t, "local", mapping1.Type)
		require.NotNil(t, mapping1.Local)
		assert.Equal(t, "src/main/java/org/example/rideshare", mapping1.Local.Path)

		// Check second mapping (github)
		mapping2 := config.SourceCode.Mappings[1]
		assert.Equal(t, "java", mapping2.Path)
		assert.Equal(t, "github", mapping2.Type)
		require.NotNil(t, mapping2.GitHub)
		assert.Equal(t, "openjdk", mapping2.GitHub.Owner)

		// Check third mapping (github)
		mapping3 := config.SourceCode.Mappings[2]
		assert.Equal(t, "org/springframework/http", mapping3.Path)
		assert.Equal(t, "github", mapping3.Type)
		require.NotNil(t, mapping3.GitHub)
		assert.Equal(t, "spring-projects", mapping3.GitHub.Owner)
	})

	t.Run("invalid - missing language", func(t *testing.T) {
		yaml := `source_code:
  mappings:
    - path: GOROOT
      type: github
      github:
        owner: golang
        repo: go
        ref: go1.24.8
        path: src
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "language is required")
	})

	t.Run("invalid - missing github config", func(t *testing.T) {
		yaml := `source_code:
  language: go
  mappings:
    - path: GOROOT
      type: github
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "github configuration is required")
	})

	t.Run("invalid - missing local path", func(t *testing.T) {
		yaml := `source_code:
  language: java
  mappings:
    - path: org/example
      type: local
      local:
        path: ""
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "local.path is required")
	})

	t.Run("invalid - unsupported type", func(t *testing.T) {
		yaml := `source_code:
  language: go
  mappings:
    - path: GOROOT
      type: gitlab
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported type")
	})

	t.Run("invalid yaml syntax", func(t *testing.T) {
		yaml := `source_code:
  language: go
  mappings:
    - path: GOROOT
      type: github
      github:
        owner: golang
        repo: go
        ref: go1.24.8
        path: src
      type: github
`
		_, err := ParsePyroscopeConfig([]byte(yaml))
		require.Error(t, err)
	})
}

func TestFindMapping(t *testing.T) {
	config := &PyroscopeConfig{
		SourceCode: SourceCodeConfig{
			Language: "java",
			Mappings: []MappingConfig{
				{
					Path: "org/example/rideshare",
					Type: "local",
					Local: &LocalMappingConfig{
						Path: "src/main/java/org/example/rideshare",
					},
				},
				{
					Path: "java",
					Type: "github",
					GitHub: &GitHubMappingConfig{
						Owner: "openjdk",
						Repo:  "jdk",
						Ref:   "jdk-17+0",
						Path:  "src/java.base/share/classes/java",
					},
				},
				{
					Path: "org/springframework/http",
					Type: "github",
					GitHub: &GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.20",
						Path:  "spring-web/src/main/java/org/springframework/http",
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		path           string
		expectedPath   string
		expectedType   string
		shouldBeNil    bool
	}{
		{
			name:         "exact match for local",
			path:         "org/example/rideshare",
			expectedPath: "org/example/rideshare",
			expectedType: "local",
		},
		{
			name:         "subdirectory of local",
			path:         "org/example/rideshare/App.java",
			expectedPath: "org/example/rideshare",
			expectedType: "local",
		},
		{
			name:         "exact match for java",
			path:         "java",
			expectedPath: "java",
			expectedType: "github",
		},
		{
			name:         "subdirectory of java",
			path:         "java/util/ArrayList.java",
			expectedPath: "java",
			expectedType: "github",
		},
		{
			name:         "exact match for springframework",
			path:         "org/springframework/http",
			expectedPath: "org/springframework/http",
			expectedType: "github",
		},
		{
			name:         "subdirectory of springframework",
			path:         "org/springframework/http/HttpStatus.java",
			expectedPath: "org/springframework/http",
			expectedType: "github",
		},
		{
			name:         "longest prefix match",
			path:         "org/springframework/http/converter/HttpMessageConverter.java",
			expectedPath: "org/springframework/http",
			expectedType: "github",
		},
		{
			name:        "no match",
			path:        "com/google/common/collect/Lists.java",
			shouldBeNil: true,
		},
		{
			name:        "partial prefix should not match",
			path:        "organization/test/File.java",
			shouldBeNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.FindMapping(tt.path)
			if tt.shouldBeNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedPath, result.Path)
				assert.Equal(t, tt.expectedType, result.Type)
			}
		})
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefix   string
		expected bool
	}{
		{
			name:     "exact match",
			path:     "java",
			prefix:   "java",
			expected: true,
		},
		{
			name:     "valid prefix with separator",
			path:     "java/util/ArrayList.java",
			prefix:   "java",
			expected: true,
		},
		{
			name:     "invalid prefix - partial word",
			path:     "javascript/test.js",
			prefix:   "java",
			expected: false,
		},
		{
			name:     "valid nested prefix",
			path:     "org/springframework/http/HttpStatus.java",
			prefix:   "org/springframework",
			expected: true,
		},
		{
			name:     "path shorter than prefix",
			path:     "org",
			prefix:   "org/springframework",
			expected: false,
		},
		{
			name:     "empty prefix",
			path:     "java/util/ArrayList.java",
			prefix:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasPrefix(tt.path, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}
