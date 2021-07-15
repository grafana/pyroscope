package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	golog "log"
	"net/http"
	"net/http/pprof"
	"os"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/markbates/pkger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/hyperloglog"
)

type Controller struct {
	drained uint32

	config     *config.Server
	storage    *storage.Storage
	httpServer *http.Server

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus
}

func New(c *config.Server, s *storage.Storage) (*Controller, error) {
	appStats, err := hyperloglog.NewPlus(uint8(18))
	if err != nil {
		return nil, err
	}

	ctrl := Controller{
		config:   c,
		storage:  s,
		stats:    make(map[string]int),
		appStats: appStats,
	}

	return &ctrl, nil
}

func (ctrl *Controller) mux() http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, []route{
		{"/healthz", ctrl.healthz},
		{"/metrics", promhttp.Handler().ServeHTTP},
		{"/config", ctrl.configHandler},
		{"/build", ctrl.buildHandler},
	})

	// drainable routes:
	routes := []route{
		{"/", ctrl.indexHandler()},
		{"/ingest", ctrl.ingestHandler},
		{"/render", ctrl.renderHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
	}

	addRoutes(mux, routes, ctrl.drainMiddleware)

	if !ctrl.config.DisablePprofEndpoint {
		addRoutes(mux, []route{
			{"/debug/pprof/", pprof.Index},
			{"/debug/pprof/cmdline", pprof.Cmdline},
			{"/debug/pprof/profile", pprof.Profile},
			{"/debug/pprof/symbol", pprof.Symbol},
			{"/debug/pprof/trace", pprof.Trace},
		})
	}
	return mux
}

func (ctrl *Controller) Start() error {
	logger := logrus.New()
	w := logger.Writer()
	defer w.Close()

	ctrl.httpServer = &http.Server{
		Addr:           ctrl.config.APIBindAddr,
		Handler:        ctrl.mux(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       golog.New(w, "", 0),
	}

	// ListenAndServe always returns a non-nil error. After Shutdown or Close,
	// the returned error is ErrServerClosed.
	err := ctrl.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (ctrl *Controller) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return ctrl.httpServer.Shutdown(ctx)
}

func (ctrl *Controller) Drain() {
	atomic.StoreUint32(&ctrl.drained, 1)
}

func (ctrl *Controller) drainMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ctrl.drained) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (ctrl *Controller) writeInvalidParameterError(w http.ResponseWriter, err error) {
	ctrl.writeError(w, http.StatusBadRequest, err, "invalid parameter")
}

func (ctrl *Controller) writeInternalServerError(w http.ResponseWriter, err error, msg string) {
	ctrl.writeError(w, http.StatusInternalServerError, err, msg)
}

func (ctrl *Controller) writeJSONEncodeError(w http.ResponseWriter, err error) {
	ctrl.writeInternalServerError(w, err, "encoding response body")
}

func (ctrl *Controller) writeError(w http.ResponseWriter, code int, err error, msg string) {
	logrus.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func (ctrl *Controller) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	logrus.Error(msg)
	writeMessage(w, code, msg)
}

func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}

type indexPage struct {
	InitialState  string
	BuildInfo     string
	ExtraMetadata string
	BaseURL       string
}

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	var dir http.FileSystem
	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more information
		dir = pkger.Dir("/webapp/public")
	} else {
		dir = http.Dir("./webapp/public")
	}
	fs := http.FileServer(dir)
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else if r.URL.Path == "/comparison" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
	}
}

func (ctrl *Controller) renderIndexPage(dir http.FileSystem, w http.ResponseWriter, _ *http.Request) {
	f, err := dir.Open("/index.html")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not find file index.html")
		return
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not read file index.html")
		return
	}

	tmpl, err := template.New("index.html").Parse(string(b))
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not parse index.html template")
		return
	}

	initialStateObj := indexPageJSON{}
	ctrl.storage.GetValues("__name__", func(v string) bool {
		initialStateObj.AppNames = append(initialStateObj.AppNames, v)
		return true
	})
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}

	initialStateStr := string(b)
	var extraMetadataStr string
	extraMetadataPath := os.Getenv("PYROSCOPE_EXTRA_METADATA")
	if extraMetadataPath != "" {
		b, err = ioutil.ReadFile(extraMetadataPath)
		if err != nil {
			logrus.Errorf("failed to read file at %s", extraMetadataPath)
		}
		extraMetadataStr = string(b)
	}

	w.Header().Add("Content-Type", "text/html")
	err = tmpl.Execute(w, indexPage{
		InitialState:  initialStateStr,
		BuildInfo:     build.JSON(),
		ExtraMetadata: extraMetadataStr,
		BaseURL:       ctrl.config.BaseURL,
	})
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not render index page template")
		return
	}
}
