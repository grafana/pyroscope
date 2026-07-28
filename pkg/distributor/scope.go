package distributor

import "regexp"

const unknownScopeName = "unknown"

var scopeVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

func sanitizeScopeForUsage(name, version string) (string, string) {
	switch name {
	case "com.grafana.pyroscope/go",
		"com.grafana.pyroscope/godeltaprof",
		"com.grafana.pyroscope/java",
		"com.grafana.pyroscope/dotnet",
		"com.grafana.pyroscope/rust",
		"com.grafana.pyroscope/python",
		"com.grafana.pyroscope/ruby",
		"com.grafana.pyroscope/nodejs",
		"com.grafana.alloy/pyroscope.scrape",
		"com.grafana.alloy/pyroscope.ebpf",
		"com.grafana.alloy/pyroscope.java":
	default:
		name = unknownScopeName
	}

	if !scopeVersionPattern.MatchString(version) {
		version = ""
	}

	return name, version
}
