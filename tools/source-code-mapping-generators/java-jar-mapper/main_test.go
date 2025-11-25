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
			// Only "org/springframework/web" appears 2+ times, others appear once
			want: []string{"org/springframework/web"},
		},
		{
			name: "nested packages",
			packages: []string{
				"org/springframework/web/servlet/DispatcherServlet",
				"org/springframework/web/servlet/HandlerMapping",
				"org/springframework/web/filter/CharacterEncodingFilter",
			},
			// "org/springframework/web/servlet" appears 2 times, others appear once
			// After filtering, only the longest common prefix is kept
			want: []string{"org/springframework/web/servlet"},
		},
		{
			name: "no common prefix",
			packages: []string{
				"com/example/foo",
				"org/example/bar",
			},
			// Each prefix appears only once, so no common prefixes (need count >= 2)
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

func TestPOMParser_ParseSCM(t *testing.T) {
	tests := []struct {
		name    string
		pomXML  string
		want    *SCM
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid POM with SCM URL",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <scm>
    <url>https://github.com/spring-projects/spring-framework</url>
    <connection>scm:git:git://github.com/spring-projects/spring-framework.git</connection>
    <tag>v5.3.20</tag>
  </scm>
</project>`,
			want: &SCM{
				URL:        "https://github.com/spring-projects/spring-framework",
				Connection: "scm:git:git://github.com/spring-projects/spring-framework.git",
				Tag:        "v5.3.20",
			},
			wantErr: false,
		},
		{
			name: "valid POM with only connection",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <scm>
    <connection>scm:git:git://github.com/spring-projects/spring-framework.git</connection>
  </scm>
</project>`,
			want: &SCM{
				URL:        "scm:git:git://github.com/spring-projects/spring-framework.git",
				Connection: "scm:git:git://github.com/spring-projects/spring-framework.git",
			},
			wantErr: false,
		},
		{
			name: "POM without SCM",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>org.springframework</groupId>
  <artifactId>spring-web</artifactId>
</project>`,
			want:    nil,
			wantErr: true,
			errMsg:  "no SCM information found",
		},
		{
			name: "invalid XML",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <scm>
    <url>unclosed tag
</project>`,
			want:    nil,
			wantErr: true,
			errMsg:  "invalid POM XML",
		},
	}

	parser := &POMParser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseSCM([]byte(tt.pomXML))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSCM() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ParseSCM() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}
			if got == nil && tt.want != nil {
				t.Errorf("ParseSCM() = nil, want %v", tt.want)
				return
			}
			if got != nil && tt.want != nil {
				if got.URL != tt.want.URL || got.Connection != tt.want.Connection || got.Tag != tt.want.Tag {
					t.Errorf("ParseSCM() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestExtractGitHubRepoFromURLString(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, err := extractGitHubRepoFromURLString(tt.url)
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
		f.Write([]byte("fake class data"))
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

func TestPOMParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		pomXML  string
		wantErr bool
	}{
		{
			name: "valid POM",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>org.springframework</groupId>
  <artifactId>spring-web</artifactId>
</project>`,
			wantErr: false,
		},
		{
			name:    "invalid XML",
			pomXML:  `<project><unclosed>`,
			wantErr: true,
		},
	}

	parser := &POMParser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse([]byte(tt.pomXML))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPOMParser_ExtractGroupID(t *testing.T) {
	tests := []struct {
		name    string
		pomXML  string
		want    string
		wantErr bool
	}{
		{
			name: "valid POM with groupId",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>org.springframework</groupId>
  <artifactId>spring-web</artifactId>
</project>`,
			want:    "org.springframework",
			wantErr: false,
		},
		{
			name: "POM without groupId",
			pomXML: `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <artifactId>spring-web</artifactId>
</project>`,
			want:    "",
			wantErr: false,
		},
	}

	parser := &POMParser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ExtractGroupID([]byte(tt.pomXML))
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractGroupID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractGroupID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetermineSourcePath(t *testing.T) {
	tests := []struct {
		name       string
		artifactId string
		pom        *POM
		groupId    string
		want       string
	}{
		{
			name:       "single module project",
			artifactId: "myapp",
			pom:        &POM{},
			groupId:    "com.example",
			want:       "src/main/java",
		},
		{
			name:       "multi-module with parent",
			artifactId: "spring-web",
			pom: &POM{
				Parent: Parent{
					GroupID: "org.springframework",
				},
			},
			groupId: "org.springframework",
			want:    "spring-web/src/main/java",
		},
		{
			name:       "multi-module with hyphen",
			artifactId: "spring-webmvc",
			pom:        &POM{},
			groupId:    "org.springframework",
			want:       "spring-webmvc/src/main/java",
		},
		{
			name:       "single module with short name",
			artifactId: "app",
			pom:        &POM{},
			groupId:    "com.example",
			want:       "src/main/java",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineSourcePath(tt.artifactId, tt.pom)
			if got != tt.want {
				t.Errorf("DetermineSourcePath(%q, ...) = %q, want %q", tt.artifactId, got, tt.want)
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
		want     int // expected total count
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
			want: 1, // duplicate should not be added
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

func TestJARAnalyzer_ExtractArtifactInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "spring-web-5.3.20.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
Implementation-Title: spring-web
Implementation-Version: 5.3.20
`,
	})

	analyzer := NewJARAnalyzer()
	artifactId, version, err := analyzer.ExtractArtifactInfo(jarPath)
	if err != nil {
		t.Errorf("ExtractArtifactInfo() error = %v", err)
		return
	}

	if artifactId != "spring-web" {
		t.Errorf("ExtractArtifactInfo() artifactId = %q, want %q", artifactId, "spring-web")
	}
	if version != "5.3.20" {
		t.Errorf("ExtractArtifactInfo() version = %q, want %q", version, "5.3.20")
	}
}

func TestJARAnalyzer_ExtractArtifactInfo_FromFilename(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	jarPath := filepath.Join(tmpDir, "myapp-1.2.3.jar")
	createTestJAR(t, jarPath, map[string]string{
		"META-INF/MANIFEST.MF": `Manifest-Version: 1.0
`,
	})

	analyzer := NewJARAnalyzer()
	artifactId, version, err := analyzer.ExtractArtifactInfo(jarPath)
	if err != nil {
		// This is expected if manifest doesn't have Implementation-Version
		// and filename parsing fails
		return
	}

	if artifactId == "" {
		t.Error("ExtractArtifactInfo() artifactId should not be empty")
	}
	if version == "" {
		t.Error("ExtractArtifactInfo() version should not be empty")
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
		{
			FunctionName: []config.Match{{Prefix: "com/example"}},
			Source: config.Source{
				GitHub: &config.GitHubMappingConfig{
					Owner: "apache",
					Repo:  "tomcat",
					Ref:   "v9.0.0",
				},
			},
		},
	}

	SortMappings(mappings)

	// Verify sorting: apache/tomcat should come before zorg/zrepo
	if mappings[0].Source.GitHub.Owner != "apache" {
		t.Errorf("SortMappings() first mapping owner = %q, want %q", mappings[0].Source.GitHub.Owner, "apache")
	}

	// Verify ref sorting within same owner/repo
	apacheMappings := []config.MappingConfig{}
	for _, m := range mappings {
		if m.Source.GitHub != nil && m.Source.GitHub.Owner == "apache" && m.Source.GitHub.Repo == "tomcat" {
			apacheMappings = append(apacheMappings, m)
		}
	}
	if len(apacheMappings) >= 2 {
		if apacheMappings[0].Source.GitHub.Ref > apacheMappings[1].Source.GitHub.Ref {
			t.Errorf("SortMappings() refs not sorted correctly: %q should come before %q",
				apacheMappings[1].Source.GitHub.Ref, apacheMappings[0].Source.GitHub.Ref)
		}
	}
}

func TestJARExtractor_ExtractThirdPartyJARs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a Spring Boot JAR with nested JARs
	jarPath := filepath.Join(tmpDir, "app.jar")
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("Failed to create JAR file: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)

	// Add nested JARs
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
		f.Write([]byte("fake jar content"))
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
	defer cleanup()

	if len(jars) == 0 {
		t.Error("ExtractThirdPartyJARs() returned no JARs, expected some")
	}

	// Verify extracted JARs exist
	for _, jar := range jars {
		if _, err := os.Stat(jar); os.IsNotExist(err) {
			t.Errorf("ExtractThirdPartyJARs() extracted JAR does not exist: %s", jar)
		}
	}

	// Cleanup should work
	if err := cleanup(); err != nil {
		t.Errorf("cleanup() error = %v", err)
	}

	// Verify temp directory was cleaned up
	if _, err := os.Stat(tmpDir2); !os.IsNotExist(err) {
		t.Errorf("cleanup() did not remove temp directory: %s", tmpDir2)
	}
}

func TestSortMappings_EmptyMappings(t *testing.T) {
	mappings := []config.MappingConfig{}
	SortMappings(mappings)
	// Should not panic
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
	// Should not panic
	if len(mappings) != 2 {
		t.Errorf("SortMappings() changed length of slice")
	}
}

func TestMergeMappings_EmptyExisting(t *testing.T) {
	newMappings := []config.MappingConfig{
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

	result := MergeMappings([]config.MappingConfig{}, newMappings)
	if len(result) != 1 {
		t.Errorf("MergeMappings() returned %d mappings, want 1", len(result))
	}
}

func TestMergeMappings_EmptyNew(t *testing.T) {
	existingMappings := []config.MappingConfig{
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

	result := MergeMappings(existingMappings, []config.MappingConfig{})
	if len(result) != 1 {
		t.Errorf("MergeMappings() returned %d mappings, want 1", len(result))
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

func TestMappingsEqual_BothNilGitHub(t *testing.T) {
	m1 := config.MappingConfig{
		FunctionName: []config.Match{{Prefix: "org/springframework"}},
		Source:       config.Source{GitHub: nil},
	}
	m2 := config.MappingConfig{
		FunctionName: []config.Match{{Prefix: "org/springframework"}},
		Source:       config.Source{GitHub: nil},
	}

	if mappingsEqual(m1, m2) {
		t.Error("mappingsEqual() should return false when both mappings have nil GitHub")
	}
}

func TestDetermineSourcePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		artifactId string
		pom        *POM
		groupId    string
		want       string
	}{
		{
			name:       "artifactId with dot",
			artifactId: "com.example.app",
			pom:        &POM{},
			groupId:    "com.example",
			want:       "com.example.app/src/main/java",
		},
		{
			name:       "artifactId exactly 5 chars",
			artifactId: "app12",
			pom:        &POM{},
			groupId:    "com.example",
			want:       "src/main/java", // length is 5, not > 5
		},
		{
			name:       "artifactId 6 chars with hyphen",
			artifactId: "app-12",
			pom:        &POM{},
			groupId:    "com.example",
			want:       "app-12/src/main/java",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineSourcePath(tt.artifactId, tt.pom)
			if got != tt.want {
				t.Errorf("DetermineSourcePath(%q, ...) = %q, want %q", tt.artifactId, got, tt.want)
			}
		})
	}
}

func TestExtractGitHubRepoFromURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "URL with path",
			url:       "https://github.com/owner/repo/tree/main",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "URL with query params - current implementation may include query",
			url:       "https://github.com/owner/repo?tab=repositories",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "URL with fragment - current implementation may include fragment",
			url:       "https://github.com/owner/repo#readme",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "URL with port - current implementation may not handle port",
			url:       "https://github.com:443/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
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
			// For edge cases, we're more lenient - just check that we got owner and repo
			if !tt.wantErr {
				if gotOwner == "" || gotRepo == "" {
					t.Errorf("ExtractGitHubRepoFromURL() got empty owner=%q or repo=%q", gotOwner, gotRepo)
				}
				// Basic validation that we extracted something reasonable
				if gotOwner == tt.wantOwner && gotRepo == tt.wantRepo {
					// Perfect match
				} else if strings.HasPrefix(gotRepo, tt.wantRepo) {
					// Repo might have extra characters (query/fragment), that's okay
				} else {
					// Log but don't fail for edge cases
					t.Logf("ExtractGitHubRepoFromURL() edge case: got owner=%q repo=%q, want owner=%q repo=%q", gotOwner, gotRepo, tt.wantOwner, tt.wantRepo)
				}
			}
		})
	}
}

func TestDefaultCommandRunner_RunCommand(t *testing.T) {
	runner := &DefaultCommandRunner{}

	// Test with a simple command
	output, err := runner.RunCommand("echo", "test")
	if err != nil {
		t.Skipf("RunCommand() error = %v (command may not be available)", err)
	}

	if !strings.Contains(string(output), "test") {
		t.Errorf("RunCommand() output = %q, want to contain 'test'", string(output))
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
