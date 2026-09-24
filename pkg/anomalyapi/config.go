package anomalyapi

import "flag"

type Config struct {
	URL string `yaml:"url" category:"experimental"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.URL, "anomaly-api.url", "",
		"Base URL of an externally configured anomaly source, used for the \"stacktrace\" "+
			"anomaly query type. Leave empty to disable.")
}
