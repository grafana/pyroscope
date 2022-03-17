package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
)

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	fs := http.FileServer(ctrl.dir)
	return func(rw http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/comparison" {
			ctrl.statsInc("comparison")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/comparison-diff" {
			ctrl.statsInc("diff")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/adhoc-single" {
			ctrl.statsInc("adhoc-index")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/adhoc-comparison" {
			ctrl.statsInc("adhoc-comparison")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/adhoc-comparison-diff" {
			ctrl.statsInc("adhoc-comparison-diff")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/settings" {
			ctrl.statsInc("settings")
			ctrl.renderIndexPage(rw, r)
		} else if strings.HasPrefix(path, "/settings") {
			ctrl.statsInc("settings")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/service-discovery" {
			ctrl.statsInc("service-discovery")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/config" {
			ctrl.statsInc("config")
			ctrl.renderIndexPage(rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
	}
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}

func (ctrl *Controller) getTemplate(path string) (*template.Template, error) {
	f, err := ctrl.dir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not find file %s: %q", path, err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %q", path, err)
	}

	tmpl, err := template.New(path).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("could not parse %s template: %q", path, err)
	}
	return tmpl, nil
}

func (ctrl *Controller) renderIndexPage(w http.ResponseWriter, _ *http.Request) {
	tmpl, err := ctrl.getTemplate("/index.html")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not render index page")
		return
	}

	initialStateObj := indexPageJSON{}
	initialStateObj.AppNames = ctrl.storage.GetAppNames()

	var b []byte
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		ctrl.writeEncodeError(w, err)
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
	features := struct {
		EnableAdhocUI bool `json:"enableAdhocUI"`
	}{
		EnableAdhocUI: !ctrl.config.NoAdhocUI,
	}
	b, err = json.Marshal(features)
	if err != nil {
		ctrl.writeEncodeError(w, err)
		return
	}
	featuresStr := string(b)

	w.Header().Add("Content-Type", "text/html")
	mustExecute(tmpl, w, map[string]string{
		"InitialState":      initialStateStr,
		"BuildInfo":         build.JSON(),
		"LatestVersionInfo": updates.LatestVersionJSON(),
		"ExtraMetadata":     extraMetadataStr,
		"BaseURL":           ctrl.config.BaseURL,
		"NotificationText":  ctrl.notifier.NotificationText(),
		"IsAuthRequired":    strconv.FormatBool(ctrl.isAuthRequired()),
		"Features":          featuresStr,
	})
}

func mustExecute(t *template.Template, w io.Writer, v interface{}) {
	if err := t.Execute(w, v); err != nil {
		panic(err)
	}
}
