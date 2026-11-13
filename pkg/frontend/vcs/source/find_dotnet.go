package source

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/v2/pkg/frontend/vcs/config"
)

const (
	ExtCSharp = ".cs"
	ExtFSharp = ".fs"
	ExtVB     = ".vb"
)

// isDotNetExtension returns true if the file extension is a C#, F# or Visual Basic extension.
func isDotNetExtension(ext string) bool {
	switch ext {
	case ExtCSharp, ExtFSharp, ExtVB:
		return true
	default:
		return false
	}
}

// findDotNetFile finds a .NET source file in a VCS repository.
// It supports path mappings from .pyroscope.yaml to map build-time PDB paths to source paths.
func (ff FileFinder) findDotNetFile(ctx context.Context, mappings ...*config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	sp, ctx := tracing.StartSpanFromContext(ctx, "findDotNetFile")
	defer sp.Finish()
	sp.SetTag("file.function_name", ff.file.FunctionName)
	sp.SetTag("file.path", ff.file.Path)

	// Profilers without PDB paths report the assembly name (e.g. "MyApp.Web") as the filename.
	if !isDotNetExtension(filepath.Ext(ff.file.Path)) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no source file path in profile, file not resolvable"))
	}

	for _, m := range mappings {
		// Strip the matched prefix, e.g. "/src/Web/Program.cs" with prefix "/src" yields "Web/Program.cs".
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
