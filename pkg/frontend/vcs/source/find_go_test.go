package source

import (
	"context"
	"testing"

	giturl "github.com/kubescape/go-git-url"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

func Test_tryFindGoFile(t *testing.T) {
	pyroscopeRepo, _ := giturl.NewGitURL("http://github.com/grafana/pyroscope")
	tests := []struct {
		name                  string
		searchedPath          string
		rootPath              string
		repo                  giturl.IGitURL
		clientMock            *mockVCSClient
		attempts              int
		expectedSearchedPaths []string
		expectedError         error
	}{
		{
			name:                  "happy case in root path",
			searchedPath:          "/var/service1/src/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            newMockVCSClient().addFiles(newFile("main.go")),
			attempts:              5,
			expectedSearchedPaths: []string{"var/service1/src/main.go", "service1/src/main.go", "src/main.go", "main.go"},
			expectedError:         nil,
		},
		{
			name:                  "happy case in submodule",
			searchedPath:          "/src/main.go",
			rootPath:              "service/example",
			repo:                  pyroscopeRepo,
			clientMock:            newMockVCSClient().addFiles(newFile("service/example/main.go")),
			attempts:              5,
			expectedSearchedPaths: []string{"service/example/src/main.go", "service/example/main.go"},
			expectedError:         nil,
		},
		{
			name:                  "path with relative repository prefix",
			searchedPath:          "github.com/grafana/pyroscope/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            newMockVCSClient().addFiles(newFile("main.go")),
			attempts:              1,
			expectedSearchedPaths: []string{"main.go"},
			expectedError:         nil,
		},
		{
			name:                  "path with absolute repository prefix",
			searchedPath:          "/Users/pyroscope/git/github.com/grafana/pyroscope/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            newMockVCSClient().addFiles(newFile("main.go")),
			attempts:              1,
			expectedSearchedPaths: []string{"main.go"},
			expectedError:         nil,
		},
		{
			name:                  "not found, attempts exceeded",
			searchedPath:          "/var/service1/src/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            newMockVCSClient().addFiles(newFile("main.go")),
			attempts:              3,
			expectedSearchedPaths: []string{"var/service1/src/main.go", "service1/src/main.go", "src/main.go"},
			expectedError:         client.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sut := FileFinder{
				file:     config.FileSpec{Path: tt.searchedPath},
				rootPath: tt.rootPath,
				ref:      defaultRef(""),
				repo:     tt.repo,
				client:   tt.clientMock,
			}
			_, err := sut.tryFindGoFile(ctx, tt.attempts)
			assert.Equal(t, tt.expectedSearchedPaths, (*tt.clientMock).searchedSequence)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}
