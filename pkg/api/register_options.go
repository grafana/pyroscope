package api

import (
	"net/http"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/middleware"

	"github.com/grafana/pyroscope/pkg/util/body"
	"github.com/grafana/pyroscope/pkg/util/delayhandler"
	"github.com/grafana/pyroscope/pkg/util/gziphandler"
	"github.com/grafana/pyroscope/pkg/validation"
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

func (a *API) WithArtificialDelayMiddleware(limits delayhandler.Limits) RegisterOption {
	return func(r *registerParams) {
		r.middlewares = append(r.middlewares, registerMiddleware{middleware.Func(delayhandler.NewHTTP(limits)), "artificial_delay"})
	}
}

func (a *API) WithBodySizeLimitMiddleware(limits body.Limits) RegisterOption {
	return func(r *registerParams) {
		r.middlewares = append(r.middlewares, registerMiddleware{middleware.Func(body.NewSizeLimitHandler(limits)), "body_size_limit"})
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

func (a *API) registerOptionsWritePath(limits *validation.Overrides) []RegisterOption {
	return []RegisterOption{
		a.WithAuthMiddleware(),
		a.WithArtificialDelayMiddleware(limits), // This middleware relies on the auth middleware, to determine the user's override
		a.WithBodySizeLimitMiddleware(limits),
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

func registerRoute(logger log.Logger, mux *mux.Router, path string, handler http.Handler, registerOpts ...RegisterOption) {
	opts := applyRegisterOptions(registerOpts...)

	level.Debug(logger).Log(append([]interface{}{
		"msg", "api: registering route"}, opts.logFields(path)...)...)

	// handle path prefixing
	route := mux.Path(path)
	if opts.isPrefix {
		route = mux.PathPrefix(path)
	}

	// limit the route to the given methods
	if len(opts.methods) > 0 {
		route = route.Methods(opts.methods...)
	}

	// registering the middlewares in reverse order (similar like middleware.Merge)
	for idx := range opts.middlewares {
		mw := opts.middlewares[len(opts.middlewares)-idx-1]
		handler = mw.Wrap(handler)
	}

	route.Handler(handler)
}
