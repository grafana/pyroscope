package admin

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

type Server struct {
	log  *logrus.Logger
	ctrl *Controller
	Mux  *http.ServeMux

	HTTPServer
}

type HTTPServer interface {
	Start(*http.ServeMux) error
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

	mux := http.NewServeMux()
	as.Mux = mux

	// Routes
	// TODO maybe use gorilla?
	mux.HandleFunc("/v1/apps", as.ctrl.HandleApps)

	return as, nil
}

func (as *Server) Start() error {
	return as.HTTPServer.Start(as.Mux)
}

func (as *Server) Stop() error {
	return as.HTTPServer.Stop()
}
