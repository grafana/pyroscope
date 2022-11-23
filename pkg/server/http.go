package server

import (
	"net/http"

	"github.com/gorilla/mux"
)

type route struct {
	pattern string
	handler http.HandlerFunc
}

func (ctrl *Controller) addRoutes(router *mux.Router, routes []route,
	middleware ...mux.MiddlewareFunc) {
	for _, r := range routes {
		router.HandleFunc(r.pattern, ctrl.trackMetrics(r.pattern)(chain(r.handler, middleware...)).ServeHTTP)
	}
}

func chain(f http.Handler, middleware ...mux.MiddlewareFunc) http.Handler {
	if len(middleware) == 0 {
		return f
	}
	return middleware[0](chain(f, middleware[1:cap(middleware)]...))
}
