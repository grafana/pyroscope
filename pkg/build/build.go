// Package build contains build-related variables set at compile time.
package build

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

var (
	Version = "N/A"
	ID      = "N/A"
	Time    = "N/A"

	GitSHA      = "N/A"
	GitDirtyStr = "-1"
	GitDirty    int

	UseEmbeddedAssetsStr = "false"
	UseEmbeddedAssets    bool
)

func init() {
	GitDirty, _ = strconv.Atoi(GitDirtyStr)
	UseEmbeddedAssets = UseEmbeddedAssetsStr == "true"
}

const tmplt = `
GENERAL
  GOARCH:             %s
  GOOS:               %s
  Version:            %s
  Build ID:           %s
  Build Time:         %s
  Git SHA:            %s
  Git Dirty Files:    %d
  Embedded Assets:    %t

AGENT
  Supported Spies:    %q
`

func Summary() string {
	return fmt.Sprintf(strings.TrimSpace(tmplt),
		runtime.GOARCH,
		runtime.GOOS,
		Version,
		ID,
		Time,
		GitSHA,
		GitDirty,
		UseEmbeddedAssets,
		spy.SupportedSpies,
	)
}

type buildInfoJSON struct {
	GOOS              string   `json:"goos"`
	GOARCH            string   `json:"goarch"`
	Version           string   `json:"version"`
	ID                string   `json:"id"`
	Time              string   `json:"time"`
	GitSHA            string   `json:"gitSHA"`
	GitDirty          int      `json:"gitDirty"`
	UseEmbeddedAssets bool     `json:"useEmbeddedAssets"`
	SupportedSpies    []string `json:"supportedSpies"`
}

func toJSONString(pretty bool) string {
	buildInfoObj := buildInfoJSON{
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
		Version:           Version,
		ID:                ID,
		Time:              Time,
		GitSHA:            GitSHA,
		GitDirty:          GitDirty,
		UseEmbeddedAssets: UseEmbeddedAssets,
		SupportedSpies:    spy.SupportedSpies,
	}
	var b []byte
	if pretty {
		b, _ = json.MarshalIndent(buildInfoObj, "", "  ")
	} else {
		b, _ = json.Marshal(buildInfoObj)
	}
	return string(b)
}

func JSON() string {
	return toJSONString(false)
}

func PrettyJSON() string {
	return toJSONString(true)
}
