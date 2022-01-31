package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/authz"
)

type Services struct {
	api.AuthService
	api.UserService
	api.APIKeyService
}

type Router struct {
	logger   logrus.FieldLogger
	redirect http.HandlerFunc
	*mux.Router
	Services

	middleware []mux.MiddlewareFunc
}

func New(logger logrus.FieldLogger, m *mux.Router, s Services) *Router {
	return &Router{
		logger:   logger,
		Router:   m,
		Services: s,
	}
}

// Use appends the given middleware to the chain.
// The handler is to be called in the order it was added.
func (r *Router) Use(middleware mux.MiddlewareFunc) *Router {
	r.middleware = append(r.middleware, middleware)
	return r
}

func (r *Router) RegisterHandlers() {
	r.registerUserHandlers()
	r.registerAPIKeyHandlers()
}

type route struct {
	path    string
	method  string
	handler http.HandlerFunc
}

// register registers given routers for the given prefix pattern.
// Authorization handler must be provided explicitly and put at the
// end of the middleware chain (invoked last).
func (r *Router) register(authorize mux.MiddlewareFunc, prefix string, routes ...route) {
	rr := r.PathPrefix(prefix).Subrouter()
	for _, x := range routes {
		h := chain(authorize(x.handler), r.middleware...)
		rr.NewRoute().Path(x.path).Methods(x.method).
			HandlerFunc(h.ServeHTTP)
	}
}

func chain(f http.Handler, middleware ...mux.MiddlewareFunc) http.Handler {
	if len(middleware) > 0 {
		return middleware[0](chain(f, middleware[1:cap(middleware)]...))
	}
	return f
}

func (r *Router) registerUserHandlers() *Router {
	h := api.NewUserHandler(r.UserService)

	r.register(authz.RequireAdminRole, "/users",
		route{"", http.MethodPost, h.CreateUser},
		route{"", http.MethodGet, h.ListUsers},
	)

	r.register(authz.RequireAdminRole, "/users/{id:[0-9]+}",
		route{"", http.MethodGet, h.GetUser},
		route{"", http.MethodPatch, h.UpdateUser},
		route{"", http.MethodDelete, h.DeleteUser},
		route{"/password", http.MethodPut, h.ChangeUserPassword},
		route{"/disable", http.MethodPut, h.DisableUser},
		route{"/enable", http.MethodPut, h.EnableUser},
		route{"/role", http.MethodPut, h.ChangeUserRole},
	)

	// Endpoints available to all authenticated users.
	r.register(authz.RequireAuthenticatedUser, "/user",
		route{"", http.MethodGet, h.GetAuthenticatedUser},
		route{"", http.MethodPatch, h.UpdateAuthenticatedUser},
		route{"/password", http.MethodPut, h.ChangeAuthenticatedUserPassword},
	)

	return r
}

func (r *Router) registerAPIKeyHandlers() *Router {
	h := api.NewAPIKeyHandler(r.APIKeyService)

	r.register(authz.RequireAdminRole, "/keys",
		route{"", http.MethodPost, h.CreateAPIKey},
		route{"", http.MethodGet, h.ListAPIKeys},
	)

	r.register(authz.RequireAdminRole, "/keys/{id:[0-9]+}",
		route{"", http.MethodDelete, h.DeleteAPIKey},
	)

	return r
}
