package source

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
)

// MockVCSClient is a mock implementation of VCSClient
type MockVCSClient struct {
	mock.Mock
}

func (m *MockVCSClient) GetFile(ctx context.Context, req client.FileRequest) (client.File, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(client.File), args.Error(1)
}

func TestConfigAwareFileFinder_WithLocalMapping(t *testing.T) {
	mockClient := new(MockVCSClient)
	logger := log.NewNopLogger()

	configYAML := `source_code:
  language: java
  mappings:
    - path: org/example/rideshare
      type: local
      local:
        path: src/main/java/org/example/rideshare
`

	// Mock .pyroscope.yaml file
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  ".pyroscope.yaml",
		Ref:   "HEAD",
	}).Return(client.File{
		Content: configYAML,
		URL:     "https://github.com/testowner/testrepo/blob/HEAD/.pyroscope.yaml",
	}, nil)

	// Mock the actual file request with mapped path
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  "src/main/java/org/example/rideshare/App.java",
		Ref:   "HEAD",
	}).Return(client.File{
		Content: "public class App { }",
		URL:     "https://github.com/testowner/testrepo/blob/HEAD/src/main/java/org/example/rideshare/App.java",
	}, nil)

	repo, err := giturl.NewGitURL("https://github.com/testowner/testrepo")
	require.NoError(t, err)

	finder := NewConfigAwareFileFinder(
		mockClient,
		repo,
		"org/example/rideshare/App.java",
		"",
		"HEAD",
		http.DefaultClient,
		logger,
	)

	result, err := finder.Find(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.URL, "src/main/java/org/example/rideshare/App.java")

	mockClient.AssertExpectations(t)
}

func TestConfigAwareFileFinder_WithGitHubMapping(t *testing.T) {
	mockClient := new(MockVCSClient)
	logger := log.NewNopLogger()

	configYAML := `source_code:
  language: java
  mappings:
    - path: java
      type: github
      github:
        owner: openjdk
        repo: jdk
        ref: jdk-17+0
        path: src/java.base/share/classes/java
`

	// Mock .pyroscope.yaml file
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  ".pyroscope.yaml",
		Ref:   "HEAD",
	}).Return(client.File{
		Content: configYAML,
		URL:     "https://github.com/testowner/testrepo/blob/HEAD/.pyroscope.yaml",
	}, nil)

	// Mock the actual file request to the mapped GitHub repo
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "openjdk",
		Repo:  "jdk",
		Path:  "src/java.base/share/classes/java/util/ArrayList.java",
		Ref:   "jdk-17+0",
	}).Return(client.File{
		Content: "package java.util; public class ArrayList { }",
		URL:     "https://github.com/openjdk/jdk/blob/jdk-17+0/src/java.base/share/classes/java/util/ArrayList.java",
	}, nil)

	repo, err := giturl.NewGitURL("https://github.com/testowner/testrepo")
	require.NoError(t, err)

	finder := NewConfigAwareFileFinder(
		mockClient,
		repo,
		"java/util/ArrayList.java",
		"",
		"HEAD",
		http.DefaultClient,
		logger,
	)

	result, err := finder.Find(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.URL, "openjdk/jdk")
	assert.Contains(t, result.URL, "src/java.base/share/classes/java/util/ArrayList.java")

	mockClient.AssertExpectations(t)
}

func TestConfigAwareFileFinder_NoConfigFallback(t *testing.T) {
	mockClient := new(MockVCSClient)
	logger := log.NewNopLogger()

	// Mock .pyroscope.yaml file not found
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  ".pyroscope.yaml",
		Ref:   "HEAD",
	}).Return(client.File{}, client.ErrNotFound)

	// Mock the actual file request (fallback to original path)
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  "src/App.java",
		Ref:   "HEAD",
	}).Return(client.File{
		Content: "public class App { }",
		URL:     "https://github.com/testowner/testrepo/blob/HEAD/src/App.java",
	}, nil)

	repo, err := giturl.NewGitURL("https://github.com/testowner/testrepo")
	require.NoError(t, err)

	finder := NewConfigAwareFileFinder(
		mockClient,
		repo,
		"src/App.java",
		"",
		"HEAD",
		http.DefaultClient,
		logger,
	)

	result, err := finder.Find(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.URL, "src/App.java")

	mockClient.AssertExpectations(t)
}

func TestConfigAwareFileFinder_NoMatchingMappingFallback(t *testing.T) {
	mockClient := new(MockVCSClient)
	logger := log.NewNopLogger()

	configYAML := `source_code:
  language: java
  mappings:
    - path: org/example
      type: local
      local:
        path: src/main/java/org/example
`

	// Mock .pyroscope.yaml file
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  ".pyroscope.yaml",
		Ref:   "HEAD",
	}).Return(client.File{
		Content: configYAML,
		URL:     "https://github.com/testowner/testrepo/blob/HEAD/.pyroscope.yaml",
	}, nil)

	// Mock the actual file request (fallback because no matching mapping)
	mockClient.On("GetFile", mock.Anything, client.FileRequest{
		Owner: "testowner",
		Repo:  "testrepo",
		Path:  "com/google/Foo.java",
		Ref:   "HEAD",
	}).Return(client.File{
		Content: "public class Foo { }",
		URL:     "https://github.com/testowner/testrepo/blob/HEAD/com/google/Foo.java",
	}, nil)

	repo, err := giturl.NewGitURL("https://github.com/testowner/testrepo")
	require.NoError(t, err)

	finder := NewConfigAwareFileFinder(
		mockClient,
		repo,
		"com/google/Foo.java",
		"",
		"HEAD",
		http.DefaultClient,
		logger,
	)

	result, err := finder.Find(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.URL, "com/google/Foo.java")

	mockClient.AssertExpectations(t)
}
