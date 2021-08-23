package server

import "net/http"

type route struct {
	pattern string
	handler http.HandlerFunc
}

func addRoutes(mux *http.ServeMux, metricsTracker metricsMiddleware, routes []route, m ...middleware) {
	for _, r := range routes {
		trackMetrics := metricsTracker(r.pattern)
		mux.HandleFunc(r.pattern, trackMetrics(chain(r.handler, m...)))
	}
}

// the metrics middleware needs to be explicit passed
// since it requires access to the pattern string
// otherwise it would infer route from the url, which would explode the cardinality
type metricsMiddleware func(name string) func(http.HandlerFunc) http.HandlerFunc
type middleware func(http.HandlerFunc) http.HandlerFunc

func chain(f http.HandlerFunc, m ...middleware) http.HandlerFunc {
	if len(m) == 0 {
		return f
	}
	return m[0](chain(f, m[1:cap(m)]...))
}
