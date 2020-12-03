package server

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/url"
	"regexp"
	"time"

	_ "net/http/pprof"

	"github.com/markbates/pkger"
	"github.com/petethepig/pyroscope/pkg/build"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/structs/sortedmap"
	log "github.com/sirupsen/logrus"
)

func init() {
	// pkger.Include("/webapp")
}

var globalMultiplier = 1000

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

var labelsRegexp *regexp.Regexp

func init() {
	labelsRegexp = regexp.MustCompile(`labels\[(.+?)\]`)
}

func labels(queryMap url.Values) *sortedmap.SortedMap {
	sortedMap := sortedmap.New()
	for k, v := range queryMap {
		res := labelsRegexp.FindStringSubmatch(k)
		if len(res) == 2 {
			sortedMap.Put(res[1], v)
		}
	}
	return sortedMap
}

func (ctrl *Controller) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", ctrl.ingestHandler)
	mux.HandleFunc("/render", ctrl.renderHandler)
	mux.HandleFunc("/labels", ctrl.labelsHandler)
	mux.HandleFunc("/label-values", ctrl.labelValuesHandler)
	var fs http.Handler
	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more context
		fs = http.FileServer(pkger.Dir("/webapp/public"))
	} else {
		fs = http.FileServer(http.Dir("./webapp/public"))
	}
	mux.HandleFunc("/", fs.ServeHTTP)
	s := &http.Server{
		Addr:           ctrl.cfg.Server.ApiBindAddr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		// ErrorLog:       log.Error.Dup(log.NewLogContext("HTTP", 1)).GoLogger(),
	}
	log.Fatal(s.ListenAndServe())
}

func compress(in []byte) []byte {
	b := bytes.Buffer{}
	gw := gzip.NewWriter(&b)
	gw.Write(in)
	gw.Close()
	return b.Bytes()
}
