package api

import (
	"strings"

	"github.com/grafana/dskit/middleware"

	"github.com/grafana/pyroscope/pkg/util/gziphandler"
)

type registerMiddleware struct {
	middleware.Interface
	name string
}

type registerParams struct {
	methods     []string
	middlewares []registerMiddleware
	isPrefix    bool
}

func (r *registerParams) logFields(path string) []interface{} {
	gzip := false
	auth := false
	for _, m := range r.middlewares {
		if m.name == "gzip" {
			gzip = true
		}
		if m.name == "auth" {
			auth = true
		}
	}
	methods := strings.Join(r.methods, ",")
	if len(r.methods) == 0 {
		methods = "all"
	}

	pathField := "path"
	if r.isPrefix {
		pathField = "prefix"
	}

	return []interface{}{
		"methods", methods,
		pathField, path,
		"auth", auth,
		"gzip", gzip,
	}
}

type RegisterOption func(*registerParams)

func applyRegisterOptions(opts ...RegisterOption) *registerParams {
	result := &registerParams{}
	for _, opt := range opts {
		opt(result)
	}
	return result
}

func WithMethod(method string) RegisterOption {
	return func(r *registerParams) {
		r.methods = append(r.methods, method)
	}
}

func WithPrefix() RegisterOption {
	return func(r *registerParams) {
		r.isPrefix = true
	}
}

func (a *API) WithAuthMiddleware() RegisterOption {
	return func(r *registerParams) {
		r.middlewares = append(r.middlewares, registerMiddleware{a.httpAuthMiddleware, "auth"})
	}
}

func WithGzipMiddleware() RegisterOption {
	return func(r *registerParams) {
		r.middlewares = append(r.middlewares, registerMiddleware{middleware.Func(gziphandler.GzipHandler), "gzip"})
	}
}

func (a *API) registerOptionsTenantPath() []RegisterOption {
	return []RegisterOption{
		a.WithAuthMiddleware(),
		WithGzipMiddleware(),
		WithMethod("GET"),
	}
}

func (a *API) registerOptionsReadPath() []RegisterOption {
	return a.registerOptionsTenantPath()
}

func (a *API) registerOptionsWritePath() []RegisterOption {
	return []RegisterOption{
		a.WithAuthMiddleware(),
		WithGzipMiddleware(),
		WithMethod("POST"),
	}
}

func (a *API) registerOptionsPublicAccess() []RegisterOption {
	return []RegisterOption{
		WithGzipMiddleware(),
		WithMethod("GET"),
	}
}

func (a *API) registerOptionsPrefixPublicAccess() []RegisterOption {
	return []RegisterOption{
		WithGzipMiddleware(),
		WithMethod("GET"),
		WithPrefix(),
	}
}

func (a *API) registerOptionsRingPage() []RegisterOption {
	return []RegisterOption{
		WithGzipMiddleware(),
		WithMethod("GET"),
		WithMethod("POST"),
	}
}
