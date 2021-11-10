package admin

import (
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
)

type Controller struct {
	log        *logrus.Logger
	svc        *AdminService
	SocketAddr string
}

type AdminServer struct {
	log        *logrus.Logger
	svc        *AdminService
	SocketAddr string
	http.Handler
}

type Config struct {
	SocketAddr string
	*logrus.Logger
}

func NewServer(c Config, svc *AdminService) (*AdminServer, error) {
	if c.SocketAddr == "" {
		return nil, fmt.Errorf("A socket path must be defined")
	}

	as := &AdminServer{
		log:        c.Logger,
		svc:        svc,
		SocketAddr: c.SocketAddr,
	}

	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/v1/apps", as.GetApps)

	as.Handler = mux

	return as, nil
}

func (as *AdminServer) Start() error {
	adminServer := http.Server{Handler: as.Handler}
	adminListener, err := net.Listen("unix", as.SocketAddr)
	if err != nil {
		return err
	}

	return adminServer.Serve(adminListener)
}

func (as *AdminServer) GetApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		{
			appNames := as.svc.GetAppNames()

			w.WriteHeader(200)
			as.writeResponseJSON(w, appNames)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}

}

func NewController(c Config, svc *AdminService) (*Controller, error) {
	if c.SocketAddr == "" {
		return nil, fmt.Errorf("A socket path must be defined")
	}

	ctrl := &Controller{
		log:        c.Logger,
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

	return adminServer.Serve(adminListener)
}

func (ctrl *Controller) GetApps(w http.ResponseWriter, r *http.Request) {
	appNames := ctrl.svc.GetAppNames()

	w.WriteHeader(200)
	ctrl.writeResponseJSON(w, appNames)
}
