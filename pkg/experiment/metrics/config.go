package metrics

import (
	"flag"
)

type Config struct {
	Enabled     bool `yaml:"enabled"`
	RulesSource struct {
		ClientAddress string `yaml:"client_address"`
	} `yaml:"rules_source"`
}

func (c *Config) Validate() error {
	return nil
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker.metrics-exporter."

	f.BoolVar(&c.Enabled, prefix+"enabled", false, "This parameter specifies whether the metrics exporter is enabled.")
	f.StringVar(&c.RulesSource.ClientAddress, prefix+"rules-source.client-address", "", "The address to use for the recording rules client connection.")
}
