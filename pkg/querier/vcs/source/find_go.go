package source

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/querier/vcs/client"
	"github.com/grafana/pyroscope/pkg/querier/vcs/source/golang"
)

const (
	ExtGo = ".go"
)

// findGoFile finds a go file in a vcs repository.
func (ff FileFinder) findGoFile(ctx context.Context) (*vcsv1.GetFileResponse, error) {
	if url, ok := golang.StandardLibraryURL(ff.path); ok {
		return ff.fetchURL(ctx, url, false)
	}

	if relativePath, ok := golang.VendorRelativePath(ff.path); ok {
		return ff.fetchRepoFile(ctx, relativePath, ff.ref)
	}

	modFile, ok := golang.ParseModuleFromPath(ff.path)
	if ok {
		mainModule := module.Version{
			Path:    path.Join(ff.repo.GetHostName(), ff.repo.GetOwnerName(), ff.repo.GetRepoName()),
			Version: module.PseudoVersion("", "", time.Time{}, ff.ref),
		}
		modf, err := ff.fetchGoMod(ctx)
		if err != nil {
			level.Warn(ff.logger).Log("msg", "failed to fetch go.mod file", "err", err)
		}
		if err := modFile.Resolve(ctx, mainModule, modf, ff.httpClient); err != nil {
			return nil, err
		}
		return ff.fetchGoDependencyFile(ctx, modFile)
	}
	return ff.tryFindGoFile(ctx, 30)
}

func (ff FileFinder) fetchGoMod(ctx context.Context) (*modfile.File, error) {
	content, err := ff.client.GetFile(ctx, client.FileRequest{
		Owner: ff.repo.GetOwnerName(),
		Repo:  ff.repo.GetRepoName(),
		Path:  golang.GoMod,
		Ref:   ff.ref,
	})
	if err != nil {
		return nil, err
	}
	return modfile.Parse(golang.GoMod, []byte(content.Content), nil)
}

func (ff FileFinder) fetchGoDependencyFile(ctx context.Context, module golang.Module) (*vcsv1.GetFileResponse, error) {
	switch {
	case module.IsGitHub():
		return ff.fetchGithubModuleFile(ctx, module)
	case module.IsGoogleSource():
		return ff.fetchGoogleSourceDependencyFile(ctx, module)
	}
	return nil, fmt.Errorf("unsupported module path: %s", module.Path)
}

func (ff FileFinder) fetchGithubModuleFile(ctx context.Context, mod golang.Module) (*vcsv1.GetFileResponse, error) {
	// todo: what if this is not a github repo?
	// 		VSClient should support querying multiple repo providers.
	githubFile, err := mod.GithubFile()
	if err != nil {
		return nil, err
	}
	content, err := ff.client.GetFile(ctx, client.FileRequest{
		Owner: githubFile.Owner,
		Repo:  githubFile.Repo,
		Path:  githubFile.Path,
		Ref:   githubFile.Ref,
	})
	if err != nil {
		return nil, err
	}
	return newFileResponse(content.Content, content.URL)
}

func (ff FileFinder) fetchGoogleSourceDependencyFile(ctx context.Context, mod golang.Module) (*vcsv1.GetFileResponse, error) {
	url, err := mod.GoogleSourceURL()
	if err != nil {
		return nil, err
	}
	return ff.fetchURL(ctx, url, true)
}

// tryFindGoFile tries to find the go file in the repo.
// It tries to find the file in the repo by removing path segment after path segment.
// maxAttempts is the maximum number of attempts to try to find the file in case the file path is very long.
// For example, if the path is "github.com/grafana/grafana/pkg/infra/log/log.go", it will try to find the file at:
// - github.com/grafana/grafana/pkg/infra/log/log.go
// - grafana/grafana/pkg/infra/log/log.go
// - pkg/infra/log/log.go
// - infra/log/log.go
// - log/log.go
// - log.go
func (ff FileFinder) tryFindGoFile(ctx context.Context, maxAttempts int) (*vcsv1.GetFileResponse, error) {
	if maxAttempts <= 0 {
		return nil, errors.New("invalid max attempts")
	}
	// Try to find the file in the repo.
	path := strings.TrimPrefix(ff.path, strings.Join([]string{ff.repo.GetHostName(), ff.repo.GetOwnerName(), ff.repo.GetRepoName()}, "/"))
	path = strings.TrimLeft(path, "/")
	attempts := 0
	for {
		content, err := ff.client.GetFile(ctx, client.FileRequest{
			Owner: ff.repo.GetOwnerName(),
			Repo:  ff.repo.GetRepoName(),
			Path:  path,
			Ref:   ff.ref,
		})
		attempts++
		if err != nil && errors.Is(err, client.ErrNotFound) && attempts < maxAttempts {
			i := strings.Index(path, "/")
			if i < 0 {
				return nil, err
			}
			// remove the first path segment
			path = path[i+1:]
			continue
		}
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)
	}
}
