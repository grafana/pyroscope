package source

import (
	"context"
	"testing"

	giturl "github.com/kubescape/go-git-url"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
)

type VCSClientMock struct {
	fileToFind       string
	searchedSequence []string
}

func (c *VCSClientMock) GetFile(ctx context.Context, req client.FileRequest) (client.File, error) {
	c.searchedSequence = append(c.searchedSequence, req.Path)
	if req.Path == c.fileToFind {
		return client.File{}, nil
	} else {
		return client.File{}, client.ErrNotFound
	}
}

func Test_tryFindGoFile(t *testing.T) {
	pyroscopeRepo, _ := giturl.NewGitURL("http://github.com/grafana/pyroscope")
	tests := []struct {
		name                  string
		searchedPath          string
		rootPath              string
		repo                  giturl.IGitURL
		clientMock            *VCSClientMock
		attempts              int
		expectedSearchedPaths []string
		expectedError         error
	}{
		{
			name:                  "happy case in root path",
			searchedPath:          "/var/service1/src/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            &VCSClientMock{fileToFind: "/main.go"},
			attempts:              5,
			expectedSearchedPaths: []string{"/var/service1/src/main.go", "/service1/src/main.go", "/src/main.go", "/main.go"},
			expectedError:         nil,
		},
		{
			name:                  "happy case in submodule",
			searchedPath:          "/src/main.go",
			rootPath:              "service/example",
			repo:                  pyroscopeRepo,
			clientMock:            &VCSClientMock{fileToFind: "service/example/main.go"},
			attempts:              5,
			expectedSearchedPaths: []string{"service/example/src/main.go", "service/example/main.go"},
			expectedError:         nil,
		},
		{
			name:                  "path with repository preffix",
			searchedPath:          "github.com/grafana/pyroscope/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            &VCSClientMock{fileToFind: "/main.go"},
			attempts:              1,
			expectedSearchedPaths: []string{"/main.go"},
			expectedError:         nil,
		},
		{
			name:                  "not found, attempts exceeded",
			searchedPath:          "/var/service1/src/main.go",
			rootPath:              "",
			repo:                  pyroscopeRepo,
			clientMock:            &VCSClientMock{fileToFind: "/main.go"},
			attempts:              3,
			expectedSearchedPaths: []string{"/var/service1/src/main.go", "/service1/src/main.go", "/src/main.go"},
			expectedError:         client.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctxMock := context.Context(nil)
			sut := FileFinder{
				path:     tt.searchedPath,
				rootPath: tt.rootPath,
				repo:     tt.repo,
				client:   tt.clientMock,
			}
			_, err := sut.tryFindGoFile(ctxMock, tt.attempts)
			assert.Equal(t, tt.expectedSearchedPaths, (*tt.clientMock).searchedSequence)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}
