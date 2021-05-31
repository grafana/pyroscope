package cli

import "path/filepath"

func defaultAgentConfigPath() string {
	return filepath.Join(getInstallPrefix(), `Pyroscope Agent`, "agent.yml")
}

func getInstallPrefix() string {
	return `C:\Program Files\Pyroscope`
}
