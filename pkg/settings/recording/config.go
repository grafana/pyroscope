package recording

import (
	"errors"
	"flag"
	"time"
)

type Config struct {
	Enabled              bool          `yaml:"enabled" category:"experimental"`
	IndexRebuildInterval time.Duration `yaml:"index_rebuild_interval" category:"advanced"`
}

const (
	flagPrefix               = "tenant-settings.recording-rules."
	flagEnabled              = flagPrefix + "enabled"
	flagIndexRebuildInterval = flagPrefix + "index-rebuild-interval"
)

func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(
		&cfg.Enabled,
		flagEnabled,
		false,
		"Enable the storing of recording rules in tenant settings.",
	)
	fs.DurationVar(
		&cfg.IndexRebuildInterval,
		flagIndexRebuildInterval,
		time.Minute,
		"How often the index of tenants with stored recording rules is rebuilt. Distributors read the index to know which tenants have rules.",
	)
}

func (cfg *Config) Validate() error {
	if cfg.Enabled && cfg.IndexRebuildInterval <= 0 {
		return errors.New("recording rules index rebuild interval must be positive")
	}
	return nil
}
