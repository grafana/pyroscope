package metrics

import (
	"flag"
)

type Config struct {
	Enabled       bool   `yaml:"enabled"`
	ClientAddress string `yaml:"client_address"`
}

func (c *Config) Validate() error {
	return nil
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker.metrics-exporter."

	f.BoolVar(&c.Enabled, prefix+"enabled", false, "This parameter specifies whether the metrics exporter is enabled.")
	f.StringVar(&c.ClientAddress, prefix+"rules-source.client-address", "", "The address to use for the recording rules client connection.")
}
