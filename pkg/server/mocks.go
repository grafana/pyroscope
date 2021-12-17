package server

import (
	"net/http"

	"github.com/gorilla/mux"
)

type mockNotifier struct{}

func (mockNotifier) NotificationText() string { return "" }

type mockAdhocServer struct{}

func (mockAdhocServer) AddRoutes(r *mux.Router) http.HandlerFunc {
	return r.ServeHTTP
}
