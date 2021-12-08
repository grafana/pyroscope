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
	registerUserHandlers(r.PathPrefix("/users").Subrouter(), s)
	return r
}

func registerUserHandlers(r *mux.Router, s *Services) {
	h := NewUserHandler(s.UserService)
	registerRoutes(r, []route{
		{"/", http.MethodPost, h.CreateUser},
		{"/", http.MethodGet, h.GetUsers},
		{"/" + patternID, http.MethodGet, h.GetUser},
		{"/" + patternID, http.MethodPut, h.UpdateUser},
		{"/" + patternID, http.MethodDelete, h.DeleteUser},
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
		return 0, errParamIDRequired
	}
	id, err := strconv.ParseUint(v, 10, 0)
	if err != nil {
		return 0, errParamIDInvalid
	}
	return uint(id), nil
}
