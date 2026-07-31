package metrics

import (
	"errors"
	"flag"
)

type Config struct {
	Enabled     bool `yaml:"enabled" category:"advanced"`
	RulesSource struct {
		ClientAddress string `yaml:"client_address" category:"advanced"`
	} `yaml:"rules_source"`
	RemoteWriteAddress string `yaml:"remote_write_address" category:"advanced"`
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

	f.BoolVar(&c.Enabled, prefix+"enabled", false, "Deprecated: use recording-rules.enabled. Specifies whether the metrics exporter is enabled.")
	f.StringVar(&c.RulesSource.ClientAddress, prefix+"rules-source.client-address", "", "Deprecated: use recording-rules.client-address. The address to use for the recording rules client connection.")
	f.StringVar(&c.RemoteWriteAddress, prefix+"remote-write-address", "", "The address to use for metrics tenant.")
}

// RecordingRulesConfig is the shared configuration for generating metrics from
// profiles via recording rules. It is read by the query-frontend (feature
// flag), the distributor (rule-aware stripping of sampled-out profiles) and the
// compaction-worker (metrics generation), so they agree on whether the feature
// is enabled and where the rules come from.
type RecordingRulesConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ClientAddress string `yaml:"client_address" category:"advanced"`
}

func (c *RecordingRulesConfig) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&c.Enabled, "recording-rules.enabled", false,
		"Enable generating metrics from profiles using recording rules. Read by the query-frontend, distributor and compaction-worker.")
	f.StringVar(&c.ClientAddress, "recording-rules.client-address", "",
		"Address of the recording rules source (the tenant-settings service). When empty, static rules from per-tenant overrides are used. Read by the distributor and compaction-worker.")
}

// ReconcileWithExporter merges the deprecated compaction-worker metrics-exporter
// settings into the shared recording-rules config. The shared values take
// precedence; the deprecated ones are honored as a fallback. Effective values
// are written back to both, so components reading either stay consistent.
func (c *RecordingRulesConfig) ReconcileWithExporter(exporter *Config) {
	if exporter.Enabled {
		c.Enabled = true
	}
	exporter.Enabled = c.Enabled

	if c.ClientAddress == "" {
		c.ClientAddress = exporter.RulesSource.ClientAddress
	}
	exporter.RulesSource.ClientAddress = c.ClientAddress
}
