package source

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

const (
	ExtJS  = ".js"
	ExtMJS = ".mjs"
	ExtCJS = ".cjs"
	ExtTS  = ".ts"
	ExtJSX = ".jsx"
	ExtTSX = ".tsx"
)

// isJavaScriptExtension returns true if the file extension is a JavaScript/TypeScript extension.
func isJavaScriptExtension(ext string) bool {
	switch ext {
	case ExtJS, ExtMJS, ExtCJS, ExtTS, ExtJSX, ExtTSX:
		return true
	default:
		return false
	}
}

// findJavaScriptFile finds a JavaScript/TypeScript file in a VCS repository.
// It supports path mappings from .pyroscope.yaml to map runtime paths to source paths.
func (ff FileFinder) findJavaScriptFile(ctx context.Context, mappings ...*config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	sp, ctx := tracing.StartSpanFromContext(ctx, "findJavaScriptFile")
	defer sp.Finish()
	sp.SetTag("file.function_name", ff.file.FunctionName)
	sp.SetTag("file.path", ff.file.Path)

	for _, m := range mappings {
		// Strip the matched prefix from the runtime path to get the relative
		// path within the mapped source (e.g., "/usr/src/app/index.js" with
		// prefix "/usr/src/app" yields "index.js").
		pos := m.Match(ff.file)
		if pos < 0 || pos > len(ff.file.Path) {
			level.Warn(ff.logger).Log("msg", "mapping match out of bounds", "pos", pos, "file_path", ff.file.Path)
			continue
		}
		path := strings.TrimLeft(ff.file.Path[pos:], "/")

		resp, err := ff.fetchMappingFile(ctx, m, path)
		if err != nil {
			if errors.Is(err, client.ErrNotFound) {
				continue
			}
			level.Warn(ff.logger).Log("msg", "failed to fetch mapping file", "err", err)
			continue
		}
		return resp, nil
	}

	// Fallback to relative file path matching
	f, err := ff.fetchRepoFile(ctx, ff.file.Path, ff.ref)
	if err != nil {
		level.Warn(ff.logger).Log("msg", "failed to fetch relative file", "err", err)
	} else {
		return f, nil
	}

	return nil, connect.NewError(connect.CodeNotFound, errors.New("no mappings matched and relative path not found, file not resolvable"))
}
