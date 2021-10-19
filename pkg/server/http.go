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

func chain(f http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	if len(middleware) == 0 {
		return f
	}
	return middleware[0](chain(f, middleware[1:cap(middleware)]...))
}
