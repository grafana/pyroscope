package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
)

type Flags struct {
	EnableAdhocUI                   bool `json:"enableAdhocUI"`
	GoogleEnabled                   bool `json:"googleEnabled"`
	GitlabEnabled                   bool `json:"gitlabEnabled"`
	GithubEnabled                   bool `json:"githubEnabled"`
	InternalAuthEnabled             bool `json:"internalAuthEnabled"`
	SignupEnabled                   bool `json:"signupEnabled"`
	ExportToFlamegraphDotComEnabled bool `json:"exportToFlamegraphDotComEnabled"`
}

type IndexHandlerConfig struct {
	Flags          Flags
	IsAuthRequired bool
	BaseURL        string
}

type IndexHandler struct {
	log       *logrus.Logger
	storage   storage.AppNameGetter
	dir       http.FileSystem
	fs        http.Handler
	stats     StatsReceiver
	notifier  Notifier
	cfg       *IndexHandlerConfig
	httpUtils httputils.Utils
}

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	cfg := &IndexHandlerConfig{
		Flags: Flags{
			EnableAdhocUI:                   !ctrl.config.NoAdhocUI,
			GoogleEnabled:                   ctrl.config.Auth.Google.Enabled,
			GitlabEnabled:                   ctrl.config.Auth.Gitlab.Enabled,
			GithubEnabled:                   ctrl.config.Auth.Github.Enabled,
			InternalAuthEnabled:             ctrl.config.Auth.Internal.Enabled,
			SignupEnabled:                   ctrl.config.Auth.Internal.SignupEnabled,
			ExportToFlamegraphDotComEnabled: !ctrl.config.DisableExportToFlamegraphDotCom,
		},
		IsAuthRequired: ctrl.isAuthRequired(),
		BaseURL:        ctrl.config.BaseURL,
	}
	return NewIndexHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.notifier, cfg, ctrl.httpUtils).ServeHTTP
}

//revive:disable:argument-limit TODO(petethepig): we will refactor this later
func NewIndexHandler(
	log *logrus.Logger,
	s storage.AppNameGetter,
	dir http.FileSystem,
	stats StatsReceiver,
	notifier Notifier,
	cfg *IndexHandlerConfig,
	httpUtils httputils.Utils,
) http.Handler {
	fs := http.FileServer(dir)
	return &IndexHandler{
		log:       log,
		storage:   s,
		dir:       dir,
		fs:        fs,
		stats:     stats,
		notifier:  notifier,
		cfg:       cfg,
		httpUtils: httpUtils,
	}
}

func (ih *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ih.stats.StatsInc("index")
	ih.renderIndexPage(w, r)
}

func (ih *IndexHandler) renderIndexPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := getTemplate(ih.dir, "/index.html")
	if err != nil {
		ih.httpUtils.WriteInternalServerError(r, w, err, "could not render index page")
		return
	}

	var b []byte
	if err != nil {
		ih.httpUtils.WriteJSONEncodeError(r, w, err)
		return
	}

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
		ih.httpUtils.WriteJSONEncodeError(r, w, err)
		return
	}
	featuresStr := string(b)

	w.Header().Add("Content-Type", "text/html")
	mustExecute(tmpl, w, map[string]string{
		"BuildInfo":         build.JSON(),
		"LatestVersionInfo": updates.LatestVersionJSON(),
		"ExtraMetadata":     extraMetadataStr,
		"BaseURL":           ih.cfg.BaseURL,
		"NotificationText":  ih.notifier.NotificationText(),
		"IsAuthRequired":    strconv.FormatBool(ih.cfg.IsAuthRequired),
		"Features":          featuresStr,
	})
}
