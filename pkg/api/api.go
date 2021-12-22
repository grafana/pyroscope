package api

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type Services struct {
	UserService
}

func Router(s *Services) *mux.Router {
	r := mux.NewRouter().PathPrefix("/api").Subrouter()
	registerUserHandlers(r, s)
	return r
}

func registerUserHandlers(r *mux.Router, s *Services) {
	h := NewUserHandler(s.UserService)

	registerRoutes(r.PathPrefix("/users").Subrouter(), []route{
		{"", http.MethodPost, h.CreateUser},
		{"", http.MethodGet, h.ListUsers},
	})

	registerRoutes(r.PathPrefix("/users/"+patternID).Subrouter(), []route{
		{"", http.MethodGet, h.GetUser},
		{"", http.MethodPatch, h.UpdateUser},
		{"/password", http.MethodPut, h.ChangeUserPassword},

		// TODO(kolesnikovae):
		//  These must not be allowed if id == owner
		//  in order to prevent self-locking scenarios.
		{"", http.MethodDelete, h.DeleteUser},
		{"/disable", http.MethodPut, h.DisableUser},
		{"/enable", http.MethodPut, h.EnableUser},
		{"/role", http.MethodPut, h.ChangeUserRoles},
	})

	// Endpoints available to authenticated users.
	registerRoutes(r.PathPrefix("/user").Subrouter(), []route{
		{"", http.MethodGet, h.GetAuthenticatedUser},
		{"", http.MethodPatch, h.UpdateAuthenticatedUser},
		{"/password", http.MethodPut, h.ChangeAuthenticatedUserPassword},
	})
}

const (
	patternID = "{id:[0-9]+}"
)

type route struct {
	path    string
	method  string
	handler http.HandlerFunc
}

func registerRoutes(r *mux.Router, routes []route) {
	for _, x := range routes {
		r.NewRoute().
			Path(x.path).
			Methods(x.method).
			HandlerFunc(x.handler)
	}
}

func idFromRequest(r *http.Request) (uint, error) {
	v, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, ErrParamIDRequired
	}
	id, err := strconv.ParseUint(v, 10, 0)
	if err != nil {
		return 0, ErrParamIDInvalid
	}
	return uint(id), nil
}
