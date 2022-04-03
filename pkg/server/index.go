package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
)

type Flags struct {
	EnableAdhocUI bool `json:"enableAdhocUI"`
}

type IndexHandlerConfig struct {
	Flags          Flags
	IsAuthRequired bool
	BaseURL        string
}

type IndexHandler struct {
	log      *logrus.Logger
	storage  storage.AppNameGetter
	dir      http.FileSystem
	fs       http.Handler
	stats    StatsReceiver
	notifier Notifier
	cfg      *IndexHandlerConfig
}

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	cfg := &IndexHandlerConfig{
		Flags: Flags{
			EnableAdhocUI: !ctrl.config.NoAdhocUI,
		},
		IsAuthRequired: ctrl.isAuthRequired(),
		BaseURL:        ctrl.config.BaseURL,
	}
	return NewIndexHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.notifier, cfg).ServeHTTP
}

//revive:disable:argument-limit TODO: we will refactor this later
func NewIndexHandler(log *logrus.Logger, s storage.AppNameGetter, dir http.FileSystem, stats StatsReceiver, notifier Notifier, cfg *IndexHandlerConfig) http.Handler {
	fs := http.FileServer(dir)
	return &IndexHandler{
		log:      log,
		storage:  s,
		dir:      dir,
		fs:       fs,
		stats:    stats,
		notifier: notifier,
		cfg:      cfg,
	}
}

func (ih *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ih.stats.StatsInc("index")
	path := r.URL.Path
	if path == "/" {
		ih.renderIndexPage(w, r)
	} else if path == "/comparison" {
		ih.renderIndexPage(w, r)
	} else if path == "/comparison-diff" {
		ih.renderIndexPage(w, r)
	} else if path == "/adhoc-single" {
		ih.renderIndexPage(w, r)
	} else if path == "/adhoc-comparison" {
		ih.renderIndexPage(w, r)
	} else if path == "/adhoc-comparison-diff" {
		ih.renderIndexPage(w, r)
	} else if path == "/settings" {
		ih.renderIndexPage(w, r)
	} else if strings.HasPrefix(path, "/settings") {
		ih.renderIndexPage(w, r)
	} else if path == "/service-discovery" {
		ih.renderIndexPage(w, r)
	} else {
		ih.fs.ServeHTTP(w, r)
	}
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}

func (ih *IndexHandler) renderIndexPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := getTemplate(ih.dir, "/index.html")
	if err != nil {
		WriteInternalServerError(ih.log, w, err, "could not render index page")
		return
	}

	initialStateObj := indexPageJSON{}
	initialStateObj.AppNames = ih.storage.GetAppNames(r.Context())

	var b []byte
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		WriteJSONEncodeError(ih.log, w, err)
		return
	}

	initialStateStr := string(b)
	var extraMetadataStr string
	extraMetadataPath := os.Getenv("PYROSCOPE_EXTRA_METADATA")
	if extraMetadataPath != "" {
		b, err = os.ReadFile(extraMetadataPath)
		if err != nil {
			logrus.Errorf("failed to read file at %s", extraMetadataPath)
		}
		extraMetadataStr = string(b)
	}

	// Feature Flags
	// Add this intermediate layer instead of just exposing as it comes from ctrl.config
	// Since we may probably want to rename these flags when exposing to the frontend
	b, err = json.Marshal(ih.cfg.Flags)
	if err != nil {
		WriteJSONEncodeError(ih.log, w, err)
		return
	}
	featuresStr := string(b)

	w.Header().Add("Content-Type", "text/html")
	mustExecute(tmpl, w, map[string]string{
		"InitialState":      initialStateStr,
		"BuildInfo":         build.JSON(),
		"LatestVersionInfo": updates.LatestVersionJSON(),
		"ExtraMetadata":     extraMetadataStr,
		"BaseURL":           ih.cfg.BaseURL,
		"NotificationText":  ih.notifier.NotificationText(),
		"IsAuthRequired":    strconv.FormatBool(ih.cfg.IsAuthRequired),
		"Features":          featuresStr,
	})
}
