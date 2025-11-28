package main

import (
	"archive/zip"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

func TestFindCommonPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		packages []string
		want     []string
	}{
		{
			name:     "empty packages",
			packages: []string{},
			want:     nil,
		},
		{
			name:     "single package",
			packages: []string{"org/springframework/web"},
			want:     nil, // Need at least 2 occurrences
		},
		{
			name: "multiple packages with common prefix",
			packages: []string{
				"org/springframework/web/HttpServlet",
				"org/springframework/web/Filter",
				"org/springframework/http/Request",
			},
			want: []string{"org/springframework/web"},
		},
		{
			name: "nested packages",
			packages: []string{
				"org/springframework/web/servlet/DispatcherServlet",
				"org/springframework/web/servlet/HandlerMapping",
				"org/springframework/web/filter/CharacterEncodingFilter",
			},
			want: []string{"org/springframework/web/servlet"},
		},
		{
			name: "no common prefix",
			packages: []string{
				"com/example/foo",
				"org/example/bar",
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findCommonPrefixes(tt.packages)
			if !slices.Equal(got, tt.want) {
				t.Errorf("findCommonPrefixes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     map[string]string
	}{
		{
			name: "simple manifest",
			manifest: `Manifest-Version: 1.0
Implementation-Title: spring-web
Implementation-Version: 5.3.20
`,
			want: map[string]string{
				"Manifest-Version":       "1.0",
				"Implementation-Title":   "spring-web",
				"Implementation-Version": "5.3.20",
			},
		},
		{
			name: "manifest with continuation lines",
			manifest: `Manifest-Version: 1.0
Implementation-Title: spring-web
Implementation-Version: 5.3.20
Bundle-Description: Spring Framework Web
 Support Classes
`,
			want: map[string]string{
				"Manifest-Version":       "1.0",
				"Implementation-Title":   "spring-web",
				"Implementation-Version": "5.3.20",
				"Bundle-Description":     "Spring Framework Web",
			},
		},
		{
			name:     "empty manifest",
			manifest: ``,
			want:     map[string]string{},
		},
		{
			name: "manifest with empty lines",
			manifest: `Manifest-Version: 1.0

Implementation-Title: spring-web

`,
			want: map[string]string{
				"Manifest-Version":     "1.0",
				"Implementation-Title": "spring-web",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseManifest(tt.manifest)
			if !maps.Equal(got, tt.want) {
				t.Errorf("parseManifest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractGitHubRepoFromURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS URL",
			url:       "https://github.com/spring-projects/spring-framework",
			wantOwner: "spring-projects",
			wantRepo:  "spring-framework",
			wantErr:   false,
		},
		{
			name:      "HTTPS URL with .git",
			url:       "https://github.com/spring-projects/spring-framework.git",
			wantOwner: "spring-projects",
			wantRepo:  "spring-framework",
			wantErr:   false,
		},
		{
			name:      "SSH URL",
			url:       "git@github.com:spring-projects/spring-framework.git",
			wantOwner: "spring-projects",
			wantRepo:  "spring-framework",
			wantErr:   false,
		},
		{
			name:      "URL with trailing slash",
			url:       "https://github.com/spring-projects/spring-framework/",
			wantOwner: "spring-projects",
			wantRepo:  "spring-framework",
			wantErr:   false,
		},
		{
			name:      "non-GitHub URL",
			url:       "https://gitlab.com/user/repo",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "invalid URL",
			url:       "not-a-url",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "SCM connection URL",
			url:       "scm:git:git@github.com:apache/spark.git",
			wantOwner: "apache",
			wantRepo:  "spark",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, err := ExtractGitHubRepoFromURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractGitHubRepoFromURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOwner != tt.wantOwner {
				t.Errorf("ExtractGitHubRepoFromURL() owner = %v, want %v", gotOwner, tt.wantOwner)
			}
			if gotRepo != tt.wantRepo {
				t.Errorf("ExtractGitHubRepoFromURL() repo = %v, want %v", gotRepo, tt.wantRepo)
			}
		})
	}
}

func TestJARAnalyzer_ExtractManifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "test.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
Implementation-Title: spring-web
Implementation-Version: 5.3.20
`,
	})

	analyzer := NewJARAnalyzer()
	got, err := analyzer.ExtractManifest(jarPath)
	if err != nil {
		t.Errorf("ExtractManifest() error = %v", err)
		return
	}

	want := map[string]string{
		"Manifest-Version":       "1.0",
		"Implementation-Title":   "spring-web",
		"Implementation-Version": "5.3.20",
	}

	if !maps.Equal(got, want) {
		t.Errorf("ExtractManifest() = %v, want %v", got, want)
	}
}

func TestJARAnalyzer_ExtractManifestMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "test.jar")
	createTestJAR(t, jarPath, map[string]string{
		"some/class.class": "fake class data",
	})

	analyzer := NewJARAnalyzer()
	_, err = analyzer.ExtractManifest(jarPath)
	if err == nil {
		t.Error("ExtractManifest() expected error for JAR without manifest")
	}
	if !strings.Contains(err.Error(), "MANIFEST.MF not found") {
		t.Errorf("ExtractManifest() error = %v, want error containing 'MANIFEST.MF not found'", err)
	}
}

func TestJARAnalyzer_ExtractPOMProperties(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// JAR filename must match artifactId in pom.properties
	jarPath := filepath.Join(tmpDir, "spark-core_2.13-4.0.1.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
`,
		"META-INF/maven/org.apache.spark/spark-core_2.13/pom.properties": `#Generated by Maven
groupId=org.apache.spark
artifactId=spark-core_2.13
version=4.0.1
`,
	})

	analyzer := NewJARAnalyzer()
	coords, err := analyzer.ExtractPOMProperties(jarPath)
	if err != nil {
		t.Errorf("ExtractPOMProperties() error = %v", err)
		return
	}

	if coords.GroupID != "org.apache.spark" {
		t.Errorf("ExtractPOMProperties() groupId = %q, want %q", coords.GroupID, "org.apache.spark")
	}
	if coords.ArtifactID != "spark-core_2.13" {
		t.Errorf("ExtractPOMProperties() artifactId = %q, want %q", coords.ArtifactID, "spark-core_2.13")
	}
	if coords.Version != "4.0.1" {
		t.Errorf("ExtractPOMProperties() version = %q, want %q", coords.Version, "4.0.1")
	}
}

func TestJARAnalyzer_ExtractPOMProperties_Missing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "test.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
`,
	})

	analyzer := NewJARAnalyzer()
	_, err = analyzer.ExtractPOMProperties(jarPath)
	if err == nil {
		t.Error("ExtractPOMProperties() expected error for JAR without pom.properties")
	}
}

func TestJARAnalyzer_ExtractPOMProperties_ShadedJAR(t *testing.T) {
	// Test that shaded JARs with mismatched pom.properties are rejected
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a JAR named "agent-2.1.2.jar" but with pom.properties from a different artifact
	jarPath := filepath.Join(tmpDir, "agent-2.1.2.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
`,
		// This pom.properties is from a shaded dependency, not the main artifact
		"META-INF/maven/org.jetbrains/annotations/pom.properties": `groupId=org.jetbrains
artifactId=annotations
version=13.0
`,
	})

	analyzer := NewJARAnalyzer()
	_, err = analyzer.ExtractPOMProperties(jarPath)
	if err == nil {
		t.Error("ExtractPOMProperties() should reject shaded JAR with mismatched pom.properties")
	}
	if !strings.Contains(err.Error(), "doesn't match JAR filename") {
		t.Errorf("ExtractPOMProperties() error should mention filename mismatch, got: %v", err)
	}
}

func TestJARAnalyzer_ExtractPOMProperties_MatchingFilename(t *testing.T) {
	// Test that pom.properties matching the JAR filename is accepted
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "spring-web-5.3.20.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
`,
		"META-INF/maven/org.springframework/spring-web/pom.properties": `groupId=org.springframework
artifactId=spring-web
version=5.3.20
`,
	})

	analyzer := NewJARAnalyzer()
	coords, err := analyzer.ExtractPOMProperties(jarPath)
	if err != nil {
		t.Errorf("ExtractPOMProperties() error = %v", err)
		return
	}
	if coords.GroupID != "org.springframework" {
		t.Errorf("ExtractPOMProperties() groupId = %q, want %q", coords.GroupID, "org.springframework")
	}
	if coords.ArtifactID != "spring-web" {
		t.Errorf("ExtractPOMProperties() artifactId = %q, want %q", coords.ArtifactID, "spring-web")
	}
}

func TestJARAnalyzer_ExtractMavenCoordinates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with pom.properties (preferred source) - filename must match artifactId
	jarPath := filepath.Join(tmpDir, "myapp-2.0.0.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
Implementation-Version: 1.0.0
`,
		"META-INF/maven/com.example/myapp/pom.properties": `groupId=com.example
artifactId=myapp
version=2.0.0
`,
	})

	analyzer := NewJARAnalyzer()
	coords, err := analyzer.ExtractMavenCoordinates(jarPath)
	if err != nil {
		t.Errorf("ExtractMavenCoordinates() error = %v", err)
		return
	}

	// Should use pom.properties values (version 2.0.0, not 1.0.0 from manifest)
	if coords.GroupID != "com.example" {
		t.Errorf("ExtractMavenCoordinates() groupId = %q, want %q", coords.GroupID, "com.example")
	}
	if coords.Version != "2.0.0" {
		t.Errorf("ExtractMavenCoordinates() version = %q, want %q", coords.Version, "2.0.0")
	}
}

func TestJARAnalyzer_ExtractMavenCoordinates_FallbackToManifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test without pom.properties (fallback to manifest)
	jarPath := filepath.Join(tmpDir, "myapp-1.0.0.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
Implementation-Version: 1.0.0
`,
	})

	analyzer := NewJARAnalyzer()
	coords, err := analyzer.ExtractMavenCoordinates(jarPath)
	if err != nil {
		t.Errorf("ExtractMavenCoordinates() error = %v", err)
		return
	}

	// Should fall back to manifest/filename
	if coords.ArtifactID != "myapp" {
		t.Errorf("ExtractMavenCoordinates() artifactId = %q, want %q", coords.ArtifactID, "myapp")
	}
	if coords.Version != "1.0.0" {
		t.Errorf("ExtractMavenCoordinates() version = %q, want %q", coords.Version, "1.0.0")
	}
	// GroupID should be empty when falling back to manifest
	if coords.GroupID != "" {
		t.Errorf("ExtractMavenCoordinates() groupId = %q, want empty", coords.GroupID)
	}
}

func TestJARAnalyzer_ExtractClassPrefixes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "test.jar")

	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("Failed to create JAR file: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)

	classes := []string{
		"org/springframework/web/HttpServlet.class",
		"org/springframework/web/Filter.class",
		"org/springframework/http/Request.class",
	}

	for _, className := range classes {
		f, err := writer.Create(className)
		if err != nil {
			t.Fatalf("Failed to create class file: %v", err)
		}
		_, err = f.Write([]byte("fake class data"))
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
	}

	writer.Close()
	file.Close()

	analyzer := NewJARAnalyzer()
	prefixes, err := analyzer.ExtractClassPrefixes(jarPath)
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			t.Skip("jar command not available")
		}
		t.Errorf("ExtractClassPrefixes() error = %v", err)
		return
	}

	if len(prefixes) == 0 {
		t.Error("ExtractClassPrefixes() returned no prefixes, expected some")
	}

	expectedPrefixes := []string{"org/springframework"}
	if !slices.Equal(prefixes, expectedPrefixes) {
		t.Logf("ExtractClassPrefixes() returned prefixes: %v (expected: %v)", prefixes, expectedPrefixes)
	}
}

func TestParseManifestContinuation(t *testing.T) {
	manifest := `Manifest-Version: 1.0
Implementation-Title: spring-web
Implementation-Version: 5.3.20
Bundle-Description: Spring Framework Web
 Support Classes
 and more text
Created-By: Apache Maven
`

	result := parseManifest(manifest)

	expected := "Spring Framework Web"
	if result["Bundle-Description"] != expected {
		t.Errorf("parseManifest() continuation line failed, got: %q, want: %q", result["Bundle-Description"], expected)
	}

	if result["Created-By"] != "Apache Maven" {
		t.Errorf("parseManifest() Created-By failed, got: %q", result["Created-By"])
	}
}

func TestFindCommonPrefixesFiltering(t *testing.T) {
	packages := []string{
		"org/springframework/web/servlet/DispatcherServlet",
		"org/springframework/web/servlet/HandlerMapping",
		"org/springframework/web/filter/CharacterEncodingFilter",
	}

	prefixes := findCommonPrefixes(packages)

	hasWebServlet := false
	for _, prefix := range prefixes {
		if prefix == "org/springframework/web/servlet" {
			hasWebServlet = true
		}
	}

	if !hasWebServlet {
		t.Error("findCommonPrefixes() should include org/springframework/web/servlet")
	}
}

func TestDetermineSourcePath(t *testing.T) {
	tests := []struct {
		name       string
		artifactId string
		want       string
	}{
		{
			name:       "single module project",
			artifactId: "myapp",
			want:       "src/main/java",
		},
		{
			name:       "multi-module with hyphen - returns empty for repo root search",
			artifactId: "spring-webmvc",
			want:       "",
		},
		{
			name:       "single module with short name",
			artifactId: "app",
			want:       "src/main/java",
		},
		{
			name:       "artifactId with hyphen returns empty",
			artifactId: "app-12",
			want:       "",
		},
		{
			name:       "Scala artifact with version suffix",
			artifactId: "spark-core_2.13",
			want:       "", // multi-module, returns empty
		},
		{
			name:       "Scala artifact without hyphen",
			artifactId: "core_2.13",
			want:       "src/main/java", // no hyphen after stripping suffix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineSourcePath(tt.artifactId)
			if got != tt.want {
				t.Errorf("determineSourcePath(%q) = %q, want %q", tt.artifactId, got, tt.want)
			}
		})
	}
}

func TestStripVersionSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"spark-core_2.13", "spark-core"},
		{"akka-actor_2.12", "akka-actor"},
		{"cats-core_2.11", "cats-core"},
		{"zio_3", "zio"},
		{"some-lib_1.0", "some-lib"},
		{"another-lib_10.2.3", "another-lib"},
		{"jackson-core", "jackson-core"}, // no suffix
		{"my-app", "my-app"},             // no suffix
		{"my_app", "my_app"},             // underscore but not version suffix
		{"lib_name_2.13", "lib_name"},    // multiple underscores, strips version
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripVersionSuffix(tt.input)
			if got != tt.want {
				t.Errorf("stripVersionSuffix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfigService_FindJarMapping(t *testing.T) {
	service := &ConfigService{}
	service.mappings = &JarMappingsConfig{
		Mappings: []JarMapping{
			{Jar: "spring-web", Owner: "spring-projects", Repo: "spring-framework", Path: "spring-web/src/main/java"},
			{Jar: "jackson-core", Owner: "FasterXML", Repo: "jackson-core", Path: "src/main/java"},
		},
	}

	tests := []struct {
		name       string
		artifactId string
		want       *JarMapping
	}{
		{
			name:       "found mapping",
			artifactId: "spring-web",
			want:       &JarMapping{Jar: "spring-web", Owner: "spring-projects", Repo: "spring-framework", Path: "spring-web/src/main/java"},
		},
		{
			name:       "not found",
			artifactId: "unknown-jar",
			want:       nil,
		},
		{
			name:       "found another mapping",
			artifactId: "jackson-core",
			want:       &JarMapping{Jar: "jackson-core", Owner: "FasterXML", Repo: "jackson-core", Path: "src/main/java"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.FindJarMapping(tt.artifactId)
			if tt.want == nil {
				if got != nil {
					t.Errorf("FindJarMapping() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("FindJarMapping() = nil, want %v", tt.want)
				return
			}
			if got.Jar != tt.want.Jar || got.Owner != tt.want.Owner || got.Repo != tt.want.Repo || got.Path != tt.want.Path {
				t.Errorf("FindJarMapping() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeMappings(t *testing.T) {
	tests := []struct {
		name     string
		existing []config.MappingConfig
		new      []config.MappingConfig
		want     int
	}{
		{
			name:     "no duplicates",
			existing: []config.MappingConfig{},
			new: []config.MappingConfig{
				{
					FunctionName: []config.Match{{Prefix: "org/springframework"}},
					Source: config.Source{
						GitHub: &config.GitHubMappingConfig{
							Owner: "spring-projects",
							Repo:  "spring-framework",
							Ref:   "v5.3.20",
						},
					},
				},
			},
			want: 1,
		},
		{
			name: "with duplicates",
			existing: []config.MappingConfig{
				{
					FunctionName: []config.Match{{Prefix: "org/springframework"}},
					Source: config.Source{
						GitHub: &config.GitHubMappingConfig{
							Owner: "spring-projects",
							Repo:  "spring-framework",
							Ref:   "v5.3.20",
						},
					},
				},
			},
			new: []config.MappingConfig{
				{
					FunctionName: []config.Match{{Prefix: "org/springframework"}},
					Source: config.Source{
						GitHub: &config.GitHubMappingConfig{
							Owner: "spring-projects",
							Repo:  "spring-framework",
							Ref:   "v5.3.20",
						},
					},
				},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeMappings(tt.existing, tt.new)
			if len(got) != tt.want {
				t.Errorf("MergeMappings() returned %d mappings, want %d", len(got), tt.want)
			}
		})
	}
}

func TestMappingsEqual(t *testing.T) {
	tests := []struct {
		name string
		m1   config.MappingConfig
		m2   config.MappingConfig
		want bool
	}{
		{
			name: "equal mappings",
			m1: config.MappingConfig{
				FunctionName: []config.Match{{Prefix: "org/springframework"}},
				Source: config.Source{
					GitHub: &config.GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.20",
					},
				},
			},
			m2: config.MappingConfig{
				FunctionName: []config.Match{{Prefix: "org/springframework"}},
				Source: config.Source{
					GitHub: &config.GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.20",
					},
				},
			},
			want: true,
		},
		{
			name: "different ref",
			m1: config.MappingConfig{
				FunctionName: []config.Match{{Prefix: "org/springframework"}},
				Source: config.Source{
					GitHub: &config.GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.20",
					},
				},
			},
			m2: config.MappingConfig{
				FunctionName: []config.Match{{Prefix: "org/springframework"}},
				Source: config.Source{
					GitHub: &config.GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.21",
					},
				},
			},
			want: false,
		},
		{
			name: "no matching prefix",
			m1: config.MappingConfig{
				FunctionName: []config.Match{{Prefix: "org/springframework"}},
				Source: config.Source{
					GitHub: &config.GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.20",
					},
				},
			},
			m2: config.MappingConfig{
				FunctionName: []config.Match{{Prefix: "com/example"}},
				Source: config.Source{
					GitHub: &config.GitHubMappingConfig{
						Owner: "spring-projects",
						Repo:  "spring-framework",
						Ref:   "v5.3.20",
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mappingsEqual(tt.m1, tt.m2)
			if got != tt.want {
				t.Errorf("mappingsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortMappings(t *testing.T) {
	mappings := []config.MappingConfig{
		{
			FunctionName: []config.Match{{Prefix: "com/example"}},
			Source: config.Source{
				GitHub: &config.GitHubMappingConfig{
					Owner: "zorg",
					Repo:  "zrepo",
					Ref:   "v1.0.0",
				},
			},
		},
		{
			FunctionName: []config.Match{{Prefix: "org/springframework"}},
			Source: config.Source{
				GitHub: &config.GitHubMappingConfig{
					Owner: "apache",
					Repo:  "tomcat",
					Ref:   "v9.0.0",
				},
			},
		},
		{
			FunctionName: []config.Match{{Prefix: "org/springframework"}},
			Source: config.Source{
				GitHub: &config.GitHubMappingConfig{
					Owner: "apache",
					Repo:  "tomcat",
					Ref:   "v8.0.0",
				},
			},
		},
	}

	SortMappings(mappings)

	if mappings[0].Source.GitHub.Owner != "apache" {
		t.Errorf("SortMappings() first mapping owner = %q, want %q", mappings[0].Source.GitHub.Owner, "apache")
	}
}

func TestJARExtractor_ExtractThirdPartyJARs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "app.jar")
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("Failed to create JAR file: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)

	nestedJARs := []string{
		"BOOT-INF/lib/spring-web-5.3.20.jar",
		"BOOT-INF/lib/jackson-core-2.13.3.jar",
		"BOOT-INF/lib/other.jar",
	}

	for _, jarName := range nestedJARs {
		f, err := writer.Create(jarName)
		if err != nil {
			t.Fatalf("Failed to create nested JAR: %v", err)
		}
		_, err = f.Write([]byte("fake jar content"))
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
	}

	writer.Close()
	file.Close()

	extractor := &JARExtractor{}
	jars, tmpDir2, cleanup, err := extractor.ExtractThirdPartyJARs(jarPath)
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			t.Skip("jar command not available")
		}
		t.Fatalf("ExtractThirdPartyJARs() error = %v", err)
	}
	defer cleanup() //nolint:errcheck

	if len(jars) == 0 {
		t.Error("ExtractThirdPartyJARs() returned no JARs, expected some")
	}

	for _, jar := range jars {
		if _, err := os.Stat(jar); os.IsNotExist(err) {
			t.Errorf("ExtractThirdPartyJARs() extracted JAR does not exist: %s", jar)
		}
	}

	if err := cleanup(); err != nil {
		t.Errorf("cleanup() error = %v", err)
	}

	if _, err := os.Stat(tmpDir2); !os.IsNotExist(err) {
		t.Errorf("cleanup() did not remove temp directory: %s", tmpDir2)
	}
}

func TestSortMappings_EmptyMappings(t *testing.T) {
	mappings := []config.MappingConfig{}
	SortMappings(mappings)
	if len(mappings) != 0 {
		t.Errorf("SortMappings() changed length of empty slice")
	}
}

func TestSortMappings_NilGitHub(t *testing.T) {
	mappings := []config.MappingConfig{
		{
			FunctionName: []config.Match{{Prefix: "com/example"}},
			Source:       config.Source{GitHub: nil},
		},
		{
			FunctionName: []config.Match{{Prefix: "org/springframework"}},
			Source: config.Source{
				GitHub: &config.GitHubMappingConfig{
					Owner: "spring-projects",
					Repo:  "spring-framework",
					Ref:   "v5.3.20",
				},
			},
		},
	}

	SortMappings(mappings)
	if len(mappings) != 2 {
		t.Errorf("SortMappings() changed length of slice")
	}
}

func TestMappingsEqual_NilGitHub(t *testing.T) {
	m1 := config.MappingConfig{
		FunctionName: []config.Match{{Prefix: "org/springframework"}},
		Source:       config.Source{GitHub: nil},
	}
	m2 := config.MappingConfig{
		FunctionName: []config.Match{{Prefix: "org/springframework"}},
		Source: config.Source{
			GitHub: &config.GitHubMappingConfig{
				Owner: "spring-projects",
				Repo:  "spring-framework",
				Ref:   "v5.3.20",
			},
		},
	}

	if mappingsEqual(m1, m2) {
		t.Error("mappingsEqual() should return false when one mapping has nil GitHub")
	}
}

func TestDefaultCommandRunner_RunCommand(t *testing.T) {
	runner := &DefaultCommandRunner{}

	output, err := runner.RunCommand("echo", "test")
	if err != nil {
		t.Skipf("RunCommand() error = %v (command may not be available)", err)
	}

	if !strings.Contains(string(output), "test") {
		t.Errorf("RunCommand() output = %q, want to contain 'test'", string(output))
	}
}

func TestParsePOMProperties(t *testing.T) {
	tests := []struct {
		name string
		data string
		want *MavenCoordinates
	}{
		{
			name: "standard properties",
			data: `#Generated by Maven
groupId=org.apache.spark
artifactId=spark-core_2.13
version=4.0.1
`,
			want: &MavenCoordinates{
				GroupID:    "org.apache.spark",
				ArtifactID: "spark-core_2.13",
				Version:    "4.0.1",
			},
		},
		{
			name: "no comments",
			data: `groupId=com.example
artifactId=myapp
version=1.0.0`,
			want: &MavenCoordinates{
				GroupID:    "com.example",
				ArtifactID: "myapp",
				Version:    "1.0.0",
			},
		},
		{
			name: "with extra whitespace",
			data: `groupId = org.test
artifactId = test-lib
version = 2.0.0
`,
			want: &MavenCoordinates{
				GroupID:    "org.test",
				ArtifactID: "test-lib",
				Version:    "2.0.0",
			},
		},
		{
			name: "empty file",
			data: "",
			want: &MavenCoordinates{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePOMProperties(tt.data)
			if got.GroupID != tt.want.GroupID {
				t.Errorf("parsePOMProperties() groupId = %q, want %q", got.GroupID, tt.want.GroupID)
			}
			if got.ArtifactID != tt.want.ArtifactID {
				t.Errorf("parsePOMProperties() artifactId = %q, want %q", got.ArtifactID, tt.want.ArtifactID)
			}
			if got.Version != tt.want.Version {
				t.Errorf("parsePOMProperties() version = %q, want %q", got.Version, tt.want.Version)
			}
		})
	}
}

func TestJdkVersionToMajorVersion(t *testing.T) {
	tests := []struct {
		jdkVersion string
		want       int
	}{
		{"8", 52},
		{"11", 55},
		{"17", 61},
		{"21", 65},
		{"99", 0}, // Unknown version
	}

	for _, tt := range tests {
		t.Run(tt.jdkVersion, func(t *testing.T) {
			got := jdkVersionToMajorVersion(tt.jdkVersion)
			if got != tt.want {
				t.Errorf("jdkVersionToMajorVersion(%q) = %d, want %d", tt.jdkVersion, got, tt.want)
			}
		})
	}
}

func TestParseNextPageURL(t *testing.T) {
	tests := []struct {
		name       string
		linkHeader string
		want       string
	}{
		{
			name:       "empty header",
			linkHeader: "",
			want:       "",
		},
		{
			name:       "single next link",
			linkHeader: `<https://api.github.com/repos/apache/spark/tags?page=2>; rel="next"`,
			want:       "https://api.github.com/repos/apache/spark/tags?page=2",
		},
		{
			name:       "next and last links",
			linkHeader: `<https://api.github.com/repos/apache/spark/tags?page=2>; rel="next", <https://api.github.com/repos/apache/spark/tags?page=10>; rel="last"`,
			want:       "https://api.github.com/repos/apache/spark/tags?page=2",
		},
		{
			name:       "prev and last links only (no next)",
			linkHeader: `<https://api.github.com/repos/apache/spark/tags?page=1>; rel="prev", <https://api.github.com/repos/apache/spark/tags?page=10>; rel="last"`,
			want:       "",
		},
		{
			name:       "first, prev, next, last links",
			linkHeader: `<https://api.github.com/repos/apache/spark/tags?page=1>; rel="first", <https://api.github.com/repos/apache/spark/tags?page=4>; rel="prev", <https://api.github.com/repos/apache/spark/tags?page=6>; rel="next", <https://api.github.com/repos/apache/spark/tags?page=10>; rel="last"`,
			want:       "https://api.github.com/repos/apache/spark/tags?page=6",
		},
		{
			name:       "malformed link (missing angle brackets)",
			linkHeader: `https://api.github.com/repos/apache/spark/tags?page=2; rel="next"`,
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNextPageURL(tt.linkHeader)
			if got != tt.want {
				t.Errorf("parseNextPageURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func createTestJAR(t *testing.T, jarPath string, files map[string]string) {
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("Failed to create JAR file: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	defer writer.Close()

	for path, content := range files {
		f, err := writer.Create(path)
		if err != nil {
			t.Fatalf("Failed to create file in JAR: %v", err)
		}
		_, err = f.Write([]byte(content))
		if err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}
	}
}
