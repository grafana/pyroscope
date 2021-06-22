package server

import "net/http"

type route struct {
	pattern string
	handler http.HandlerFunc
}

func addRoutes(mux *http.ServeMux, routes []route, m ...middleware) {
	for _, r := range routes {
		mux.HandleFunc(r.pattern, chain(r.handler, m...))
	}
}

type middleware func(http.HandlerFunc) http.HandlerFunc

func chain(f http.HandlerFunc, m ...middleware) http.HandlerFunc {
	if len(m) == 0 {
		return f
	}
	return m[0](chain(f, m[1:cap(m)]...))
}
