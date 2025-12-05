package source

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

func newMockVCSClient() *mockVCSClient {
	return &mockVCSClient{
		files: make(map[client.FileRequest]client.File),
	}
}

type mockFileResponse struct {
	request client.FileRequest
	content string
}

func newFile(path string) mockFileResponse {
	return mockFileResponse{
		request: client.FileRequest{
			Path: path,
		},
		content: "# Content of " + path,
	}
}

func (f *mockFileResponse) url() string {
	return fmt.Sprintf(
		"https://github.com/%s/%s/blob/%s/%s",
		f.request.Owner,
		f.request.Repo,
		f.request.Ref,
		f.request.Path,
	)
}

type mockVCSClient struct {
	mtx              sync.Mutex
	files            map[client.FileRequest]client.File
	searchedSequence []string
}

func (c *mockVCSClient) GetFile(ctx context.Context, req client.FileRequest) (client.File, error) {
	c.mtx.Lock()
	c.searchedSequence = append(c.searchedSequence, req.Path)
	file, ok := c.files[req]
	c.mtx.Unlock()
	if ok {
		return file, nil
	}
	return client.File{}, client.ErrNotFound
}

func (c *mockVCSClient) addFiles(files ...mockFileResponse) *mockVCSClient {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for _, file := range files {
		file.request.Owner = defaultOwner(file.request.Owner)
		file.request.Repo = defaultRepo(file.request.Repo)
		file.request.Ref = defaultRef(file.request.Ref)
		c.files[file.request] = client.File{
			Content: file.content,
			URL:     file.url(),
		}
	}
	return c
}

func defaultOwner(s string) string {
	if s == "" {
		return "grafana"
	}
	return s
}

func defaultRepo(s string) string {
	if s == "" {
		return "pyroscope"
	}
	return s
}
func defaultRef(s string) string {
	if s == "" {
		return "main"
	}
	return s
}

const javaPyroscopeYAML = `---
source_code:
  mappings:
    - function_name:
        - prefix: org/example/rideshare
      language: java
      source:
        local:
          path: src/main/java
    - function_name:
        - prefix: java
      language: java
      source:
        github:
          owner: openjdk
          repo: jdk
          ref: jdk-17+0
          path: src/java.base/share/classes
    - function_name:
        - prefix: org/springframework/http
        - prefix: org/springframework/web
      language: java
      source:
        github:
          owner: spring-projects
          repo: spring-framework
          ref: v5.3.20
          path: spring-web/src/main/java
    - function_name:
        - prefix: org/springframework/web/servlet
      language: java
      source:
        github:
          owner: spring-projects
          repo: spring-framework
          ref: v5.3.20
          path: spring-webmvc/src/main/java
`

const goPyroscopeYAML = `---
source_code:
  mappings:
    - path:
        - prefix: $GOROOT/src
      language: go
      source:
       github:
        owner: golang
        repo: go
        ref: go1.24.8
        path: src
`

const goPyroscopeYAMLBazel = `---
source_code:
  mappings:
    - path:
        - prefix: external/gazelle++go_deps+com_github_stretchr_testify
      language: go
      source:
        github:
          owner: stretchr
          repo: testify
          ref: v1.10.0
`

// TestFileFinder_Find tests the complete happy path integration for find.go using table-driven tests
func TestFileFinder_Find(t *testing.T) {
	tests := []struct {
		name            string
		fileSpec        config.FileSpec
		owner           string
		repo            string
		ref             string
		rootPath        string
		pyroscopeYAML   string
		mockFiles       []mockFileResponse
		expectedContent string
		expectedURL     string
		expectedError   bool
	}{
		// Java tests
		{
			name: "java/mapped-local-path",
			fileSpec: config.FileSpec{
				FunctionName: "org/example/rideshare/RideShareController.orderCar",
			},
			rootPath:      "examples/language-sdk-instrumentation/java/rideshare",
			ref:           "main",
			pyroscopeYAML: javaPyroscopeYAML,
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Repo: "pyroscope",
						Path: "examples/language-sdk-instrumentation/java/rideshare/src/main/java/org/example/rideshare/RideShareController.java",
						Ref:  "main",
					},
					content: "# CONTENT RideShareController.java",
				},
			},
			expectedContent: "# CONTENT RideShareController.java",
			expectedURL:     "https://github.com/grafana/pyroscope/blob/main/examples/language-sdk-instrumentation/java/rideshare/src/main/java/org/example/rideshare/RideShareController.java",
			expectedError:   false,
		},
		{
			name: "java/mapped-dependency",
			fileSpec: config.FileSpec{
				FunctionName: "java/lang/Math.floorMod",
			},
			rootPath:      "examples/language-sdk-instrumentation/java/rideshare",
			ref:           "main",
			pyroscopeYAML: javaPyroscopeYAML,
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "openjdk",
						Repo:  "jdk",
						Ref:   "jdk-17+0",
						Path:  "src/java.base/share/classes/java/lang/Math.java",
					},
					content: "# CONTENT Math.java",
				},
			},
			expectedContent: "# CONTENT Math.java",
			expectedURL:     "https://github.com/openjdk/jdk/blob/jdk-17+0/src/java.base/share/classes/java/lang/Math.java",
			expectedError:   false,
		},
		// Go tests
		{
			name: "go/not-mapped-local-path",
			fileSpec: config.FileSpec{
				FunctionName: "github.com/grafana/pyroscope/pkg/compactionworker.(*Worker).runCompaction",
				Path:         "/Users/christian/git/github.com/grafana/pyroscope/pkg/compactionworker/worker.go",
			},
			ref: "main",
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "grafana",
						Repo:  "pyroscope",
						Ref:   "main",
						Path:  "pkg/compactionworker/worker.go",
					},
					content: "# CONTENT worker.go",
				},
			},
			expectedContent: "# CONTENT worker.go",
			expectedURL:     "https://github.com/grafana/pyroscope/blob/main/pkg/compactionworker/worker.go",
			expectedError:   false,
		},
		{
			name: "go/not-mapped-dependency-gomod",
			fileSpec: config.FileSpec{
				FunctionName: "github.com/parquet-go/parquet-go.(*bufferPool).newBuffer",
				Path:         "/Users/christian/.golang/packages/pkg/mod/github.com/parquet-go/parquet-go@v0.23.0/buffer.go",
			},
			ref: "main",
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Path: "go.mod",
					},
					content: `
module github.com/grafana/pyroscope

go 1.24.6

toolchain go1.24.9

require (
	github.com/parquet-go/parquet-go v0.25.0
)
`,
				},
				{
					request: client.FileRequest{
						Owner: "parquet-go",
						Repo:  "parquet-go",
						Ref:   "v0.25.0",
						Path:  "buffer.go",
					},
					content: "# CONTENT buffer.go",
				},
			},
			expectedContent: "# CONTENT buffer.go",
			expectedURL:     "https://github.com/parquet-go/parquet-go/blob/v0.25.0/buffer.go",
			expectedError:   false,
		},
		{
			name: "go/not-mapped-dependency-no-gomod-file",
			fileSpec: config.FileSpec{
				FunctionName: "github.com/parquet-go/parquet-go.(*bufferPool).newBuffer",
				// without go.mod file in the version of the dependency comes from the file path
				Path: "/Users/christian/.golang/packages/pkg/mod/github.com/parquet-go/parquet-go@v0.23.0/buffer.go",
			},
			ref: "main",
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "parquet-go",
						Repo:  "parquet-go",
						Ref:   "v0.23.0",
						Path:  "buffer.go",
					},
					content: "# CONTENT buffer.go",
				},
			},
			expectedContent: "# CONTENT buffer.go",
			expectedURL:     "https://github.com/parquet-go/parquet-go/blob/v0.23.0/buffer.go",
			expectedError:   false,
		},
		{
			name: "go/not-mapped-dependency-vendor",
			fileSpec: config.FileSpec{
				FunctionName: "github.com/grafana/loki/v3/pkg/iter/v2.(*PeekIter).cacheNext",
				Path:         "/src/enterprise-logs/vendor/github.com/grafana/loki/v3/pkg/iter/v2/iter.go",
			},
			ref:  "HEAD",
			repo: "enterprise-logs",
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "grafana",
						Repo:  "enterprise-logs",
						Ref:   "HEAD",
						Path:  "vendor/github.com/grafana/loki/v3/pkg/iter/v2/iter.go",
					},
					content: "# CONTENT iter.go",
				},
			},
			expectedContent: "# CONTENT iter.go",
			expectedURL:     "https://github.com/grafana/enterprise-logs/blob/HEAD/vendor/github.com/grafana/loki/v3/pkg/iter/v2/iter.go",
			expectedError:   false,
		},
		{
			name: "go/not-mapped-stdlib",
			fileSpec: config.FileSpec{
				FunctionName: "bufio.(*Reader).ReadSlice",
				Path:         "/usr/local/go/src/bufio/bufio.go",
			},
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "golang",
						Repo:  "go",
						Ref:   "master",
						Path:  "src/bufio/bufio.go",
					},
					content: "# CONTENT bufio.go",
				},
			},
			expectedContent: "# CONTENT bufio.go",
			expectedURL:     "https://github.com/golang/go/blob/master/src/bufio/bufio.go",
			expectedError:   false,
		},
		{
			name: "go/mapped-stdlib",
			fileSpec: config.FileSpec{
				FunctionName: "bufio.(*Reader).ReadSlice",
				Path:         "/usr/local/go/src/bufio/bufio.go",
			},
			pyroscopeYAML: goPyroscopeYAML,
			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "golang",
						Repo:  "go",
						Ref:   "go1.24.8",
						Path:  "src/bufio/bufio.go",
					},
					content: "# CONTENT bufio.go",
				},
			},
			expectedContent: "# CONTENT bufio.go",
			expectedURL:     "https://github.com/golang/go/blob/go1.24.8/src/bufio/bufio.go",
			expectedError:   false,
		},
		{
			name: "go/mapped-dependency-bazel",
			fileSpec: config.FileSpec{
				FunctionName: "github.com/stretchr/testify/require.NoError",
				Path:         "external/gazelle++go_deps+com_github_stretchr_testify/require/require.go",
			},
			pyroscopeYAML: goPyroscopeYAMLBazel,
			owner:         "bazel-contrib",
			repo:          "rules_go",
			rootPath:      "examples/basic_gazelle",
			ref:           "v0.59.0",

			mockFiles: []mockFileResponse{
				{
					request: client.FileRequest{
						Owner: "stretchr",
						Repo:  "testify",
						Ref:   "v1.10.0",
						Path:  "require/require.go",
					},
					content: "# CONTENT require.go",
				},
			},
			expectedContent: "# CONTENT require.go",
			expectedURL:     "https://github.com/stretchr/testify/blob/v1.10.0/require/require.go",
			expectedError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock VCS client
			mockClient := newMockVCSClient()

			// Populate pyroscopeYAML content into first mock file (if present)
			mockFiles := tt.mockFiles
			if tt.pyroscopeYAML != "" {
				mockFiles = append(mockFiles, mockFileResponse{
					request: client.FileRequest{
						Owner: tt.owner,
						Repo:  tt.repo,
						Ref:   tt.ref,
						Path:  filepath.Join(tt.rootPath, ".pyroscope.yaml"),
					},
					content: tt.pyroscopeYAML,
				})
			}
			mockClient.addFiles(mockFiles...)

			// Setup repository URL
			repoURL, err := giturl.NewGitURL(fmt.Sprintf("https://github.com/%s/%s", defaultOwner(tt.owner), defaultRepo(tt.repo)))
			require.NoError(t, err)

			// Create HTTP client
			httpClient := &http.Client{}

			// Create FileFinder
			finder := NewFileFinder(
				mockClient,
				repoURL,
				tt.fileSpec,
				tt.rootPath,
				defaultRef(tt.ref),
				httpClient,
				log.NewNopLogger(),
			)

			// Execute the Find method
			response, err := finder.Find(ctx)

			// Assertions
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err, "Find should succeed")
				require.NotNil(t, response, "Response should not be nil")

				// Decode and verify content
				decodedContent, err := base64.StdEncoding.DecodeString(response.Content)
				require.NoError(t, err, "Content should be valid base64")
				assert.Equal(t, tt.expectedContent, string(decodedContent), "Content should match expected file")

				// Verify URL
				assert.Equal(t, tt.expectedURL, response.URL, "URL should point to correct location")
			}
		})
	}
}
