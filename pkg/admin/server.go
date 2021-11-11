package admin

import (
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Config struct {
	SocketAddr string
	Log        *logrus.Logger
}

type Server struct {
	log  *logrus.Logger
	ctrl *Controller
	Mux  *http.ServeMux

	Config
	HTTPServer
}

type HTTPServer interface {
	Start(*http.ServeMux) error
	Stop() error
}

// NewServer creates an AdminServer and returns an error
// Is also does basic verifications:
// - Checks if the SocketAddress is non empty
func NewServer(c Config, ctrl *Controller, httpServer HTTPServer) (*Server, error) {
	if c.SocketAddr == "" {
		return nil, fmt.Errorf("A socket path must be defined")
	}

	as := &Server{
		log:    c.Log,
		ctrl:   ctrl,
		Config: c,
	}
	as.HTTPServer = httpServer

	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/check", as.ctrl.Check(as.SocketAddr))
	mux.HandleFunc("/v1/apps", as.ctrl.HandleGetApps)

	as.Mux = mux

	return as, nil
}

func (as *Server) Start() error {
	return as.HTTPServer.Start(as.Mux)
}

func (as *Server) Stop() error {
	return as.HTTPServer.Stop()
}
