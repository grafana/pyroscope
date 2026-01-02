package source

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

const (
	ExtPython = ".py"
)

var (
	// stdLibRegex matches Python stdlib paths and captures:
	//   [1] version - e.g., "3.12"
	//   [2] path    - the remaining file path after the version directory
	// Example: "/lib/python3.12/difflib.py" → version="3.12", path="difflib.py"
	stdLibRegex = regexp.MustCompile(`python(\d+\.\d{1,2})/(.+)`)
)

func (ff FileFinder) fetchPythonStdlib(ctx context.Context, path string, version string) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "fetchPythonStdlib")
	defer sp.Finish()

	// use main branch as fallback
	ref := "main"
	if version != "" {
		ref = version
	}

	content, err := ff.client.GetFile(ctx, client.FileRequest{
		Owner: "python",
		Repo:  "cpython",
		Path:  filepath.Join("Lib", path),
		Ref:   ref,
	})
	if err != nil {
		return nil, err
	}
	return newFileResponse(content.Content, content.URL)
}

// isPythonStdlibPath returns the cleaned path of the standard library package with version, if detected.
// For example, given "/path/to/lib/python3.12/difflib.py",
// it returns ("difflib.py", "3.12", true).
// Note that minor versions are not captured in this path, so there are
// future improvements that can be made to this logic.
func isPythonStdlibPath(path string) (string, string, bool) {
	matches := stdLibRegex.FindStringSubmatch(path)
	if matches == nil {
		return "", "", false
	}
	// matches[0] = full match, matches[1] = version, matches[2] = remaining path
	return matches[2], matches[1], true
}

// findPythonFile finds a python file in a vcs repository.
// Currently only supports Python stdlib
func (ff FileFinder) findPythonFile(ctx context.Context, mappings ...*config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "findPythonFile")
	defer sp.Finish()
	sp.SetTag("file.function_name", ff.file.FunctionName)
	sp.SetTag("file.path", ff.file.Path)

	if path, version, ok := isPythonStdlibPath(ff.file.Path); ok {
		return ff.fetchPythonStdlib(ctx, path, version)
	}

	for _, m := range mappings {
		// Note that this path is an absolute path that is very runtime-by-runtime dependent
		resp, err := ff.fetchMappingFile(ctx, m, ff.file.Path)
		if err != nil {
			if errors.Is(err, client.ErrNotFound) {
				continue
			}
			level.Warn(ff.logger).Log("msg", "failed to fetch mapping file", "err", err)
			continue
		}
		return resp, nil
	}

	return nil, fmt.Errorf("stdlib not detected and no mappings provided, file not resolvable")
}
