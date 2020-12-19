// Package build contains build-related variables set at compile time.
package build

import (
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

	RbSpyGitSHA      = "N/A"
	RbSpyGitDirtyStr = "-1"
	RbSpyGitDirty    int

	PySpyGitSHA      = "N/A"
	PySpyGitDirtyStr = "-1"
	PySpyGitDirty    int

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

SPIES BUILD INFO:
	* rbspy %s / %s / %d
	* pyspy %s / %s / %d
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
		RbSpyGitSHA,
		RbSpyGitDirtyStr,
		RbSpyGitDirty,
		PySpyGitSHA,
		PySpyGitDirtyStr,
		PySpyGitDirty,
	)
}
