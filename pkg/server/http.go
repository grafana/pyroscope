package server

import "net/http"

type route struct {
	pattern string
	handler http.HandlerFunc
}

func (ctrl *Controller) addRoutes(mux *http.ServeMux, routes []route,
	middleware ...func(http.HandlerFunc) http.HandlerFunc) {
	for _, r := range routes {
		mux.HandleFunc(r.pattern, ctrl.trackMetrics(r.pattern)(chain(r.handler, middleware...)))
	}
}

// the metrics middleware needs to be explicit passed
// since it requires access to the pattern string
// otherwise it would infer route from the url, which would explode the cardinality
type metricsMiddleware func(name string) func(http.HandlerFunc) http.HandlerFunc

func chain(f http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	if len(middleware) == 0 {
		return f
	}
	return middleware[0](chain(f, middleware[1:cap(middleware)]...))
}
