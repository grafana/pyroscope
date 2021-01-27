package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	golog "log"
	"net/http"
	"os"
	"sync"
	"text/template"
	"time"

	_ "net/http/pprof"

	"github.com/clarkduvall/hyperloglog"
	"github.com/markbates/pkger"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
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
	var dir http.FileSystem
	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more information
		dir = pkger.Dir("/webapp/public")
	} else {
		dir = http.Dir("./webapp/public")
	}
	fs := http.FileServer(dir)
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
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
	atexit.Register(func() {
		s.Close()
	})
	err := s.ListenAndServe()
	if err != nil {
		if err == http.ErrServerClosed {
			return
		}
		logrus.Error(err)
	}
}

func renderServerError(rw http.ResponseWriter, text string) {
	rw.WriteHeader(500)
	rw.Write([]byte(text))
	rw.Write([]byte("\n"))
}

type indexPageJson struct {
	AppNames []string `json:"appNames"`
}

type indexPage struct {
	InitialState  string
	ExtraMetadata string
}

func (ctrl *Controller) renderIndexPage(dir http.FileSystem, rw http.ResponseWriter, r *http.Request) {
	f, err := dir.Open("index.html")
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not find file index.html: %q", err))
		return
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not read file index.html: %q", err))
		return
	}

	tmpl, err := template.New("index.html").Parse(string(b))
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not parse index.html template: %q", err))
		return
	}

	jsonObj := indexPageJson{}
	ctrl.s.GetValues("__name__", func(v string) bool {
		jsonObj.AppNames = append(jsonObj.AppNames, v)
		return true
	})
	b, err = json.Marshal(jsonObj)
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not marshal json: %q", err))
		return
	}

	jsonStr := string(b)

	var extraMetadataStr string
	extraMetadataPath := os.Getenv("PYROSCOPE_EXTRA_METADATA")
	if extraMetadataPath != "" {
		b, err = ioutil.ReadFile(extraMetadataPath)
		if err != nil {
			logrus.Error("failed to read file at %s", extraMetadataPath)
		}
		extraMetadataStr = string(b)
	}

	rw.Header().Add("Content-Type", "text/html")
	rw.WriteHeader(200)
	err = tmpl.Execute(rw, indexPage{
		InitialState:  jsonStr,
		ExtraMetadata: extraMetadataStr,
	})
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not marshal json: %q", err))
		return
	}

}
