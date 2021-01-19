package server

import (
	golog "log"
	"net/http"
	"sync"
	"time"

	_ "net/http/pprof"

	"github.com/clarkduvall/hyperloglog"
	"github.com/markbates/pkger"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

func init() {
	// pkger.Include("/webapp")
}

type Controller struct {
	cfg *config.Config
	s   *storage.Storage

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus
}

func New(cfg *config.Config, s *storage.Storage) *Controller {
	appStats, _ := hyperloglog.NewPlus(uint8(18))
	return &Controller{
		cfg:      cfg,
		s:        s,
		stats:    make(map[string]int),
		appStats: appStats,
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
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		ctrl.statsInc("index")
		fs.ServeHTTP(rw, r)
	})

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
	err := s.ListenAndServe()
	if err != nil {
		logrus.Error(err)
	}
}
