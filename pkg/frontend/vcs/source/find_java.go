package source

import (
	"context"
	"fmt"
	"strings"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
	"github.com/opentracing/opentracing-go"
)

const (
	ExtJava = ".java"
)

// findJavaFile finds a java file in a vcs repository.
func (ff FileFinder) findJavaFile(ctx context.Context, mappings ...*config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "findJavaFile")
	defer sp.Finish()
	sp.SetTag("file.function_name", ff.file.FunctionName)
	sp.SetTag("file.path", ff.file.Path)

	pathSegments := strings.Split(ff.file.FunctionName, "/")
	last := len(pathSegments) - 1

	// find first dot in last segment
	posDot := strings.Index(pathSegments[last], ".")
	if posDot > 0 {
		pathSegments[last] = pathSegments[last][:posDot]
	}
	// TODO: treat $$

	pathSegments[last] = pathSegments[last] + ExtJava

	javaPath := strings.Join(pathSegments, "/")

	for _, m := range mappings {
		return ff.fetchMappingFile(ctx, m, javaPath)
	}

	return nil, fmt.Errorf("no mappings provided, file not resolvable")
}
