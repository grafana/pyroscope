package server

import (
	golog "log"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/markbates/pkger"
	"github.com/petethepig/pyroscope/pkg/build"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/storage"
	log "github.com/sirupsen/logrus"
)

func init() {
	// pkger.Include("/webapp")
}

type Controller struct {
	cfg *config.Config
	s   *storage.Storage
}

func New(cfg *config.Config, s *storage.Storage) *Controller {
	return &Controller{
		cfg: cfg,
		s:   s,
	}
}

func (ctrl *Controller) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", ctrl.ingestHandler)
	mux.HandleFunc("/render", ctrl.renderHandler)
	mux.HandleFunc("/labels", ctrl.labelsHandler)
	mux.HandleFunc("/label-values", ctrl.labelValuesHandler)
	var fs http.Handler
	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more information
		fs = http.FileServer(pkger.Dir("/webapp/public"))
	} else {
		fs = http.FileServer(http.Dir("./webapp/public"))
	}
	mux.HandleFunc("/", fs.ServeHTTP)

	logger := log.New()
	w := logger.Writer()
	defer w.Close()
	s := &http.Server{
		Addr:           ctrl.cfg.Server.ApiBindAddr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       golog.New(w, "", 0),
	}
	s.ListenAndServe()
}
