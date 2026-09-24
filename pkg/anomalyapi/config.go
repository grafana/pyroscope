// Package anomalyapi provides a thin client for an externally configured anomaly source (any
// HTTP service implementing its small anomaly-lookup API). It's off by default --
// self-hosted/OSS users who don't configure it simply can't use the "stacktrace" anomaly query
// type.
package anomalyapi

import "flag"

// Config holds the configuration for the anomaly source client.
type Config struct {
	// URL is the base URL of the anomaly source's API. Empty (the default) disables it -- any
	// query for an anomaly type backed by this client will fail with a clear error rather than
	// silently doing nothing.
	URL string `yaml:"url" category:"experimental"`
}

// RegisterFlags registers the anomaly source client's flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.URL, "anomaly-api.url", "",
		"Base URL of an externally configured anomaly source, used for the \"stacktrace\" "+
			"anomaly query type. Leave empty to disable.")
}
