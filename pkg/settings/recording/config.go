package recording

import (
	"flag"
)

type Config struct {
	Enabled bool `yaml:"enabled" category:"experimental"`
}

const (
	flagPrefix  = "tenant-settings.recording-rules."
	flagEnabled = flagPrefix + "enabled"
)

func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(
		&cfg.Enabled,
		flagEnabled,
		false,
		"Enable the storing of recording rules in tenant settings.",
	)
}

func (cfg *Config) Validate() error {
	if !cfg.Enabled {
		return nil
	}
	return nil
}
