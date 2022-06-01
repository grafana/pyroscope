//go:build !windows
// +build !windows

package cli

func defaultAgentConfigPath() string {
	return "/etc/pyroscope/agent.yml"
}

func defaultAgentLogFilePath() string { return "" }

func getInstallPrefix() string { return "" }
