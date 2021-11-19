package admin

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type Server struct {
	log     *logrus.Logger
	ctrl    *Controller
	Handler http.Handler

	HTTPServer
}

type HTTPServer interface {
	Start(http.Handler) error
	Stop() error
}

// NewServer creates an AdminServer and returns an error
// Is also does basic verifications:
// - Checks if the SocketAddress is non empty
func NewServer(logger *logrus.Logger, ctrl *Controller, httpServer HTTPServer) (*Server, error) {
	as := &Server{
		log:  logger,
		ctrl: ctrl,
	}
	as.HTTPServer = httpServer

	// use gorilla mux
	r := mux.NewRouter()

	as.Handler = r

	// Routes
	// TODO maybe use gorilla?
	r.HandleFunc("/v1/apps", as.ctrl.GetAppsHandler).Methods("GET")
	r.HandleFunc("/v1/apps", as.ctrl.DeleteAppHandler).Methods("DELETE")

	return as, nil
}

func (as *Server) Start() error {
	return as.HTTPServer.Start(as.Handler)
}

func (as *Server) Stop() error {
	return as.HTTPServer.Stop()
}
