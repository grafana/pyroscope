package router

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/authz"
)

type Services struct {
	api.UserService
	api.APIKeyService
}

type route struct {
	path    string
	method  string
	handler http.HandlerFunc
}

func New(s *Services) *mux.Router {
	r := mux.NewRouter().PathPrefix("/api").Subrouter()
	registerUserHandlers(r, s)
	registerAPIKeyHandlers(r, s)
	return r
}

type middleware func(http.HandlerFunc) http.HandlerFunc

func register(authorize middleware, r *mux.Router, routes []route, middleware ...middleware) {
	for _, x := range routes {
		r.NewRoute().Path(x.path).Methods(x.method).
			HandlerFunc(chain(authorize(x.handler), middleware...))
	}
}

func chain(f http.HandlerFunc, middleware ...middleware) http.HandlerFunc {
	if len(middleware) == 0 {
		return f
	}
	return middleware[0](chain(f, middleware[1:cap(middleware)]...))
}

func registerUserHandlers(r *mux.Router, s *Services) {
	h := api.NewUserHandler(s.UserService)

	register(authz.RequireAdminRole, r.PathPrefix("/users").Subrouter(), []route{
		{"", http.MethodPost, h.CreateUser},
		{"", http.MethodGet, h.ListUsers},
	})

	register(authz.RequireAdminRole, r.PathPrefix("/users/{id:[0-9]+}").Subrouter(), []route{
		{"", http.MethodGet, h.GetUser},
		{"", http.MethodPatch, h.UpdateUser},
		{"", http.MethodDelete, h.DeleteUser},
		{"/password", http.MethodPut, h.ChangeUserPassword},
		{"/disable", http.MethodPut, h.DisableUser},
		{"/enable", http.MethodPut, h.EnableUser},
		{"/role", http.MethodPut, h.ChangeUserRole},
	})

	// Endpoints available to all authenticated users.
	register(authz.RequireAuthenticatedUser, r.PathPrefix("/user").Subrouter(), []route{
		{"", http.MethodGet, h.GetAuthenticatedUser},
		{"", http.MethodPatch, h.UpdateAuthenticatedUser},
		{"/password", http.MethodPut, h.ChangeAuthenticatedUserPassword},
	})
}

func registerAPIKeyHandlers(r *mux.Router, s *Services) {
	h := api.NewAPIKeyHandler(s.APIKeyService)

	register(authz.RequireAdminRole, r.PathPrefix("/keys").Subrouter(), []route{
		{"", http.MethodPost, h.CreateAPIKey},
		{"", http.MethodGet, h.ListAPIKeys},
	})

	register(authz.AllowAny, r.PathPrefix("/keys/{id:[0-9]+}").Subrouter(), []route{
		{"", http.MethodDelete, h.DeleteAPIKey},
	})
}
