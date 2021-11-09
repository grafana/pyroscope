package admin

import (
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
)

// TODO get this from parameters?

type Controller struct {
	log        *logrus.Logger
	close      chan struct{}
	svc        *AdminService
	SocketAddr string
}

type Config struct {
	SocketAddr string
	*logrus.Logger
}

func NewController(c Config, svc *AdminService) (*Controller, error) {
	if c.SocketAddr == "" {
		return nil, fmt.Errorf("A socket path must be defined")
	}

	ctrl := &Controller{
		log:        c.Logger,
		close:      make(chan struct{}),
		SocketAddr: c.SocketAddr,
		svc:        svc,
	}

	return ctrl, nil
}

func (ctrl *Controller) Start() error {
	if err := os.RemoveAll(ctrl.SocketAddr); err != nil {
		return err
	}

	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/v1/apps", ctrl.GetApps)

	adminServer := http.Server{Handler: mux}
	adminListener, err := net.Listen("unix", ctrl.SocketAddr)
	if err != nil {
		return err
	}

	// TODO
	// is this blocking?
	return adminServer.Serve(adminListener)
}

func (ctrl *Controller) GetApps(w http.ResponseWriter, r *http.Request) {
	appNames := ctrl.svc.GetAppNames()

	ctrl.writeResponseJSON(w, appNames)
}
