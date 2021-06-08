package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func defaultAgentConfigPath() string {
	return filepath.Join(getInstallPrefix(), "/etc/pyroscope/agent.yml")
}

func defaultAgentLogFilePath() string { return "" }

// on mac pyroscope is usually installed via homebrew. homebrew installs under a prefix
//   this is logic to figure out what prefix it is
func getInstallPrefix() string {
	if runtime.GOOS != "darwin" {
		return ""
	}

	executablePath, err := os.Executable()
	if err != nil {
		// TODO: figure out what kind of errors might happen, handle it
		return ""
	}
	cellarPath := filepath.Clean(filepath.Join(resolvePath(executablePath), "../../../.."))

	if !strings.HasSuffix(cellarPath, "Cellar") {
		// looks like it's not installed via homebrew
		return ""
	}

	return filepath.Clean(filepath.Join(cellarPath, "../"))
}
