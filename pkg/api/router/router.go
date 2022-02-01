package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/authz"
)

type Router struct {
	logger logrus.FieldLogger
	*mux.Router
	Services
}

type Services struct {
	api.AuthService
	api.UserService
	api.APIKeyService
}

func New(logger logrus.FieldLogger, m *mux.Router, s Services) *Router {
	return &Router{
		logger:   logger,
		Router:   m,
		Services: s,
	}
}

func (r *Router) RegisterHandlers() {
	r.RegisterUserHandlers()
	r.RegisterAPIKeyHandlers()
}

func (r *Router) RegisterUserHandlers() {
	h := api.NewUserHandler(r.UserService)

	x := r.PathPrefix("/users").Subrouter()
	x.Use(authz.RequireAdminRole)
	x.Methods(http.MethodPost).HandlerFunc(h.CreateUser)
	x.Methods(http.MethodGet).HandlerFunc(h.ListUsers)

	x = x.PathPrefix("/{id:[0-9]+}").Subrouter()
	x.Methods(http.MethodGet).HandlerFunc(h.GetUser)
	x.Methods(http.MethodPatch).HandlerFunc(h.UpdateUser)
	x.Methods(http.MethodDelete).HandlerFunc(h.DeleteUser)
	x.Path("/password").Methods(http.MethodPut).HandlerFunc(h.ChangeUserPassword)
	x.Path("/disable").Methods(http.MethodPut).HandlerFunc(h.DisableUser)
	x.Path("/enable").Methods(http.MethodPut).HandlerFunc(h.EnableUser)
	x.Path("/role").Methods(http.MethodPut).HandlerFunc(h.ChangeUserRole)

	// Endpoints available to all authenticated users.
	x = r.PathPrefix("/user").Subrouter()
	x.Use(authz.RequireAuthenticatedUser)
	x.Methods(http.MethodGet).HandlerFunc(h.GetAuthenticatedUser)
	x.Methods(http.MethodPatch).HandlerFunc(h.UpdateAuthenticatedUser)
	x.Path("/password").Methods(http.MethodPut).HandlerFunc(h.ChangeAuthenticatedUserPassword)
}

func (r *Router) RegisterAPIKeyHandlers() {
	h := api.NewAPIKeyHandler(r.APIKeyService)

	x := r.PathPrefix("/keys").Subrouter()
	x.Use(authz.RequireAdminRole)
	x.Methods(http.MethodPost).HandlerFunc(h.CreateAPIKey)
	x.Methods(http.MethodGet).HandlerFunc(h.ListAPIKeys)

	x = r.PathPrefix("/{id:[0-9]+}").Subrouter()
	x.Methods(http.MethodDelete).HandlerFunc(h.DeleteAPIKey)
}
