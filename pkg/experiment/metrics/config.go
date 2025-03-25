package metrics

import (
	"errors"
	"flag"
)

type Config struct {
	Enabled     bool `yaml:"enabled"`
	RulesSource struct {
		ClientAddress string `yaml:"client_address"`
	} `yaml:"rules_source"`
	RemoteWriteAddress string `yaml:"remote_write_address"`
}

func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.RemoteWriteAddress == "" {
		return errors.New("remote write address is required")
	}
	return nil
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker.metrics-exporter."

	f.BoolVar(&c.Enabled, prefix+"enabled", false, "This parameter specifies whether the metrics exporter is enabled.")
	f.StringVar(&c.RulesSource.ClientAddress, prefix+"rules-source.client-address", "", "The address to use for the recording rules client connection.")
	f.StringVar(&c.RemoteWriteAddress, prefix+"remote-write-address", "", "The address to use for metrics tenant.")
}
