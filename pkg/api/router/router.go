package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/authz"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

type Router struct {
	*mux.Router
	Services
}

type Services struct {
	Logger logrus.FieldLogger

	api.AuthService
	api.UserService
	api.APIKeyService
	api.AnnotationsService
	api.AdhocService
	api.ApplicationListerAndDeleter
}

func New(m *mux.Router, s Services) *Router {
	return &Router{
		Router:   m,
		Services: s,
	}
}

func (r *Router) RegisterHandlers() {
	r.RegisterUserHandlers()
	r.RegisterAPIKeyHandlers()
	r.RegisterAnnotationsHandlers()
}

func (r *Router) RegisterUserHandlers() {
	h := api.NewUserHandler(r.UserService, httputils.NewDefaultHelper(r.Logger))
	authorizer := authz.NewAuthorizer(r.Services.Logger, httputils.NewDefaultHelper(r.Logger))

	x := r.PathPrefix("/users").Subrouter()
	x.Use(authorizer.RequireAdminRole)
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
	x.Use(authorizer.RequireAuthenticatedUser)
	x.Methods(http.MethodGet).HandlerFunc(h.GetAuthenticatedUser)
	x.Methods(http.MethodPatch).HandlerFunc(h.UpdateAuthenticatedUser)
	x.Path("/password").Methods(http.MethodPut).HandlerFunc(h.ChangeAuthenticatedUserPassword)
}

func (r *Router) RegisterAPIKeyHandlers() {
	h := api.NewAPIKeyHandler(r.Logger, r.APIKeyService, httputils.NewDefaultHelper(r.Logger))
	authorizer := authz.NewAuthorizer(r.Services.Logger, httputils.NewDefaultHelper(r.Logger))

	x := r.PathPrefix("/keys").Subrouter()
	x.Use(authorizer.RequireAdminRole)
	x.Methods(http.MethodPost).HandlerFunc(h.CreateAPIKey)
	x.Methods(http.MethodGet).HandlerFunc(h.ListAPIKeys)

	x = x.PathPrefix("/{id:[0-9]+}").Subrouter()
	x.Methods(http.MethodDelete).HandlerFunc(h.DeleteAPIKey)
}

func (r *Router) RegisterAnnotationsHandlers() {
	h := api.NewAnnotationsHandler(r.AnnotationsService, httputils.NewDefaultHelper(r.Logger))

	x := r.PathPrefix("/annotations").Subrouter()
	x.Methods(http.MethodPost).HandlerFunc(h.CreateAnnotation)
}

func (r *Router) RegisterAdhocHandlers(maxFileSize int64) {
	h := api.NewAdhocHandler(r.AdhocService, httputils.NewDefaultHelper(r.Logger), maxFileSize)

	x := r.PathPrefix("/adhoc/v1").Subrouter()
	x.Methods(http.MethodGet).PathPrefix("/profiles").HandlerFunc(h.GetProfiles)
	x.Methods(http.MethodGet).PathPrefix("/profile/{id:[0-9a-f]+}").HandlerFunc(h.GetProfile)
	x.Methods(http.MethodGet).PathPrefix("/diff/{left:[0-9a-f]+}/{right:[0-9a-f]+}").HandlerFunc(h.GetProfileDiff)
	x.Methods(http.MethodPost).PathPrefix("/upload").HandlerFunc(h.Upload)
}

func (r *Router) RegisterApplicationHandlers() {
	h := api.NewApplicationsHandler(r.ApplicationListerAndDeleter, httputils.NewDefaultHelper(r.Logger))
	authorizer := authz.NewAuthorizer(r.Services.Logger, httputils.NewDefaultHelper(r.Logger))

	x := r.PathPrefix("/apps").Subrouter()
	x.Methods(http.MethodGet).HandlerFunc(h.GetApps)
	x.Methods(http.MethodDelete).Handler(authorizer.RequireAdminRole(http.HandlerFunc(h.DeleteApp)))
}
