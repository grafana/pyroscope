// +build !windows

package cli

func defaultAgentConfigPath() string { return "/etc/pyroscope/agent.yml" }

func getInstallPrefix() string { return "" }
