package source

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/source/golang"
)

const (
	ExtGo  = ".go"
	ExtAsm = ".s" // Assembler files in go
)

// findGoFile finds a go file in a vcs repository.
func (ff FileFinder) findGoFile(ctx context.Context, mappings ...*config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "findGoFile")
	defer sp.Finish()
	sp.SetTag("file.path", ff.file.Path)
	sp.SetTag("file.function_name", ff.file.FunctionName)

	// if we have mappings try those first
	for _, m := range mappings {
		pos := m.Match(ff.file)
		if pos < 0 || pos > len(ff.file.Path) {
			level.Warn(ff.logger).Log("msg", "mapping cut off out of bounds", "pos", pos, "file_path", ff.file.Path)
			continue
		}
		resp, err := ff.fetchMappingFile(ctx, m, strings.TrimLeft(ff.file.Path[pos:], "/"))
		if err != nil {
			if errors.Is(err, client.ErrNotFound) {
				continue
			}
			level.Warn(ff.logger).Log("msg", "failed to fetch mapping file", "err", err)
			continue
		}
		return resp, nil
	}

	if path, version, ok := golang.IsStandardLibraryPath(ff.file.Path); ok {
		return ff.fetchGoStdlib(ctx, path, version)
	}

	if relativePath, ok := golang.VendorRelativePath(ff.file.Path); ok {
		return ff.fetchRepoFile(ctx, relativePath, ff.ref)
	}

	modFile, ok := golang.ParseModuleFromPath(ff.file.Path)
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

func (ff FileFinder) fetchGoStdlib(ctx context.Context, path string, version string) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "fetchGoStdlib")
	defer sp.Finish()

	// if there is no version detected, use the one from .pyroscope.yaml
	if version == "" && ff.config != nil {
		mapping := ff.config.FindMapping(config.FileSpec{Path: "$GOROOT/src"})
		if mapping != nil {
			return ff.fetchMappingFile(ctx, mapping, path)
		}
	}

	// use master branch as fallback
	ref := "master"
	if version != "" {
		ref = "go" + version
	}

	content, err := ff.client.GetFile(ctx, client.FileRequest{
		Owner: "golang",
		Repo:  "go",
		Path:  filepath.Join("src", path),
		Ref:   ref,
	})
	if err != nil {
		return nil, err
	}
	return newFileResponse(content.Content, content.URL)
}

func (ff FileFinder) fetchGoMod(ctx context.Context) (*modfile.File, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "fetchGoMod")
	defer sp.Finish()
	sp.SetTag("owner", ff.repo.GetOwnerName())
	sp.SetTag("repo", ff.repo.GetRepoName())
	sp.SetTag("ref", ff.ref)

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
	sp, ctx := opentracing.StartSpanFromContext(ctx, "fetchGoDependencyFile")
	defer sp.Finish()
	sp.SetTag("module_path", module.Path)

	switch {
	case module.IsGitHub():
		return ff.fetchGithubModuleFile(ctx, module)
	case module.IsGoogleSource():
		return ff.fetchGoogleSourceDependencyFile(ctx, module)
	}
	return nil, fmt.Errorf("unsupported module path: %s", module.Path)
}

func (ff FileFinder) fetchGithubModuleFile(ctx context.Context, mod golang.Module) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "fetchGithubModuleFile")
	defer sp.Finish()
	sp.SetTag("module_path", mod.Path)

	// todo: what if this is not a github repo?
	// 		VSClient should support querying multiple repo providers.
	githubFile, err := mod.GithubFile()
	if err != nil {
		return nil, err
	}
	sp.SetTag("owner", githubFile.Owner)
	sp.SetTag("repo", githubFile.Repo)
	sp.SetTag("path", githubFile.Path)
	sp.SetTag("ref", githubFile.Ref)

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
	sp, ctx := opentracing.StartSpanFromContext(ctx, "fetchGoogleSourceDependencyFile")
	defer sp.Finish()
	sp.SetTag("module_path", mod.Path)

	url, err := mod.GoogleSourceURL()
	if err != nil {
		return nil, err
	}
	sp.SetTag("url", url)
	return ff.fetchURL(ctx, url, true)
}

// tryFindGoFile tries to find the go file in the repo, under the rootPath.
// It tries to find the file in the rootPath inside the repo by removing path segment after path segment.
// maxAttempts is the maximum number of attempts to try to find the file in case the file path is very long.
// For example, if the path is "github.com/grafana/grafana/pkg/infra/log/log.go" and rootPath is "path/to/module1", it
// will try to find the file at:
// - "path/to/module1/pkg/infra/log/log.go"
// - "path/to/module1/infra/log/log.go"
// - "path/to/module1/log/log.go"
// - "path/to/module1/log.go"
func (ff FileFinder) tryFindGoFile(ctx context.Context, maxAttempts int) (*vcsv1.GetFileResponse, error) {
	if maxAttempts <= 0 {
		return nil, errors.New("invalid max attempts")
	}

	// trim repo path (e.g. "github.com/grafana/pyroscope/") in path
	path := ff.file.Path
	repoPath := strings.Join([]string{ff.repo.GetHostName(), ff.repo.GetOwnerName(), ff.repo.GetRepoName(), ""}, "/")
	if pos := strings.Index(path, repoPath); pos != -1 {
		path = path[len(repoPath)+pos:]
	}

	// now try to find file in repo
	path = strings.TrimLeft(path, "/")
	attempts := 0
	for {
		reqPath := path
		if ff.rootPath != "" {
			reqPath = strings.Join([]string{ff.rootPath, path}, "/")
		}
		content, err := ff.client.GetFile(ctx, client.FileRequest{
			Owner: ff.repo.GetOwnerName(),
			Repo:  ff.repo.GetRepoName(),
			Path:  reqPath,
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
