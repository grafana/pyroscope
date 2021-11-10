package admin

import (
	"fmt"
	"net"
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

	Config
	http.Handler
}

// NewServer creates an AdminServer and returns an error
// Is also does basic verifications:
// - Checks if the SocketAddress is non empty
func NewServer(c Config, ctrl *Controller) (*Server, error) {
	if c.SocketAddr == "" {
		return nil, fmt.Errorf("A socket path must be defined")
	}

	as := &Server{
		log:    c.Log,
		ctrl:   ctrl,
		Config: c,
	}

	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/check", as.ctrl.Check(as.SocketAddr))
	mux.HandleFunc("/v1/apps", as.ctrl.GetApps)

	as.Handler = mux

	return as, nil
}

func (as *Server) Start() error {
	adminServer := http.Server{Handler: as.Handler}

	// we listen on a unix domain socket
	// which will be created by the os
	// https://man7.org/linux/man-pages/man2/bind.2.html
	adminListener, err := net.Listen("unix", as.SocketAddr)
	if err != nil {
		return err
	}

	return adminServer.Serve(adminListener)
}
