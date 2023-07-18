package cli

import (
	"os"
	"path/filepath"
)

func defaultAgentConfigPath() string {
	return filepath.Join(getInstallPrefix(), `Pyroscope Agent`, "agent.yml")
}

func defaultAgentLogFilePath() string {
	return filepath.Join(getDataDirectory(), `Pyroscope Agent`, "pyroscope-agent.log")
}

func getInstallPrefix() string {
	return `C:\Program Files\Pyroscope`
}

func getDataDirectory() string {
	p, ok := os.LookupEnv("PROGRAMDATA")
	if ok {
		return filepath.Join(p, `Pyroscope`)
	}
	e, err := os.Executable()
	if err == nil {
		return filepath.Dir(e)
	}
	return filepath.Dir(os.Args[0])
}
