// Package build contains build-related variables set at compile time.
package build

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/webapp"
)

var (
	Version = "N/A"
	ID      = "N/A"
	Time    = "N/A"

	GitSHA      = "N/A"
	GitDirtyStr = "-1"
	GitDirty    int

	RbspyGitSHA  = "N/A"
	PyspyGitSHA  = "N/A"
	PhpspyGitSHA = "N/A"
)

func init() {
	GitDirty, _ = strconv.Atoi(GitDirtyStr)
}

const tmplt = `
GENERAL
  GOARCH:             %s
  GOOS:               %s
  Go Version:         %s
  Version:            %s
  Build ID:           %s
  Build Time:         %s
  Git SHA:            %s
  Git Dirty Files:    %d
  Embedded Assets:    %t

AGENT
  Supported Spies:    %q
  rbspy  Git SHA:     %q
  pyspy  Git SHA:     %q
  phpspy Git SHA:     %q
`

func Summary() string {
	return fmt.Sprintf(strings.TrimSpace(tmplt),
		runtime.GOARCH,
		runtime.GOOS,
		runtime.Version(),
		Version,
		ID,
		Time,
		GitSHA,
		GitDirty,
		webapp.AssetsEmbedded,
		spy.SupportedSpies,
		RbspyGitSHA,
		PyspyGitSHA,
		PhpspyGitSHA,
	)
}

type buildInfoJSON struct {
	GOOS              string `json:"goos"`
	GOARCH            string `json:"goarch"`
	GoVersion         string `json:"goVersion"`
	Version           string `json:"version"`
	ID                string `json:"id"`
	Time              string `json:"time"`
	GitSHA            string `json:"gitSHA"`
	GitDirty          int    `json:"gitDirty"`
	UseEmbeddedAssets bool   `json:"useEmbeddedAssets"`
	RbspyGitSHA       string `json:"rbspyGitSHA"`
	PyspyGitSHA       string `json:"pyspyGitSHA"`
	PhpspyGitSHA      string `json:"phpspyGitSHA"`
}

func generateBuildInfoJSON() buildInfoJSON {
	return buildInfoJSON{
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
		GoVersion:         runtime.Version(),
		Version:           Version,
		ID:                ID,
		Time:              Time,
		GitSHA:            GitSHA,
		GitDirty:          GitDirty,
		UseEmbeddedAssets: webapp.AssetsEmbedded,
		RbspyGitSHA:       RbspyGitSHA,
		PyspyGitSHA:       PyspyGitSHA,
		PhpspyGitSHA:      PhpspyGitSHA,
	}
}

func JSON() string {
	b, _ := json.Marshal(generateBuildInfoJSON())
	return string(b)
}

func PrettyJSON() string {
	b, _ := json.MarshalIndent(generateBuildInfoJSON(), "", "  ")
	return string(b)
}

// PrometheusBuildLabels returns a map of the labels
// that will be exposed in the build_info metric
func PrometheusBuildLabels() prometheus.Labels {
	return prometheus.Labels{
		"GOOS":                runtime.GOOS,
		"GOARCH":              runtime.GOARCH,
		"version":             Version,
		"time":                Time,
		"revision":            GitSHA,
		"use_embedded_assets": strconv.FormatBool(webapp.AssetsEmbedded),
	}
}
