package source

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

const (
	ExtJava = ".java"
)

func convertJavaFunctionNameToPath(functionName string) string {
	pathSegments := strings.Split(functionName, "/")
	last := len(pathSegments) - 1

	// pos to cut from
	pos := -1
	updatePos := func(v int) {
		if v == -1 {
			return
		}
		if pos == -1 || pos > v {
			pos = v
		}
	}

	// find first dot in last segment
	updatePos(strings.Index(pathSegments[last], "."))

	// find first $ in last segment
	updatePos(strings.Index(pathSegments[last], "$"))

	if pos > 0 {
		pathSegments[last] = pathSegments[last][:pos]
	}

	pathSegments[last] = pathSegments[last] + ExtJava
	return strings.Join(pathSegments, "/")
}

// findJavaFile finds a java file in a vcs repository.
func (ff FileFinder) findJavaFile(ctx context.Context, mappings ...*config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "findJavaFile")
	defer sp.Finish()
	sp.SetTag("file.function_name", ff.file.FunctionName)
	sp.SetTag("file.path", ff.file.Path)

	javaPath := convertJavaFunctionNameToPath(ff.file.FunctionName)
	for _, m := range mappings {
		resp, err := ff.fetchMappingFile(ctx, m, javaPath)
		if err != nil {
			if errors.Is(err, client.ErrNotFound) {
				continue
			}
			level.Warn(ff.logger).Log("msg", "failed to fetch mapping file", "err", err)
			continue
		}
		return resp, nil
	}

	return nil, fmt.Errorf("no mappings provided, file not resolvable")
}
