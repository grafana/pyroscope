package admin

import (
	"net/http"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"

	"github.com/gorilla/handlers"
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

	httpUtils := httputils.NewDefaultHelper(logger)
	// Routes

	applicationsHandler := api.NewApplicationsHandler(ctrl.appService, httpUtils)
	r.HandleFunc("/v1/apps", applicationsHandler.GetApps).Methods("GET")
	r.HandleFunc("/v1/apps", applicationsHandler.DeleteApp).Methods("DELETE")

	r.HandleFunc("/v1/users/{username}", ctrl.UpdateUserHandler).Methods("PATCH")
	r.HandleFunc("/v1/storage/cleanup", ctrl.StorageCleanupHandler).Methods("PUT")

	// Global middlewares
	r.Use(logginMiddleware)

	return as, nil
}

func (as *Server) Start() error {
	return as.HTTPServer.Start(as.Handler)
}

func (as *Server) Stop() error {
	return as.HTTPServer.Stop()
}

func logginMiddleware(next http.Handler) http.Handler {
	// log to Stdout using Apache Common Log Format
	// TODO maybe use JSON?
	return handlers.LoggingHandler(os.Stdout, next)
}
