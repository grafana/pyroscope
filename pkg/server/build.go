package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	"github.com/pyroscope-io/pyroscope/pkg/build"
)

type buildInfoJSON struct {
	GOOS              string `json:"goos"`
	GOARCH            string `json:"goarch"`
	Version           string `json:"version"`
	ID                string `json:"id"`
	Time              string `json:"time"`
	GitSHA            string `json:"gitSHA"`
	GitDirty          int    `json:"gitDirty"`
	UseEmbeddedAssets bool   `json:"useEmbeddedAssets"`
}

func (ctrl *Controller) buildHandler(w http.ResponseWriter, r *http.Request) {
	buildInfoStr, err := buildInfoJSONString(true)
	if err != nil {
		renderServerError(w, fmt.Sprintf("could not marshal buildInfoObj json: %q", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte(buildInfoStr))
}

func buildInfoJSONString(pretty bool) (string, error) {
	buildInfoObj := buildInfoJSON{
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
		Version:           build.Version,
		ID:                build.ID,
		Time:              build.Time,
		GitSHA:            build.GitSHA,
		GitDirty:          build.GitDirty,
		UseEmbeddedAssets: build.UseEmbeddedAssets,
	}
	var b []byte
	var err error

	if pretty {
		b, err = json.MarshalIndent(buildInfoObj, "", "  ")
	} else {
		b, err = json.Marshal(buildInfoObj)
	}

	if err != nil {
		return "", err
	}

	return string(b), nil
}
