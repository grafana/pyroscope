package recording

import (
	"flag"
	"fmt"
)

type Config struct {
	Enabled bool   `yaml:"enabled" category:"experimental"`
	Address string `yaml:"address" category:"experimental"`
}

const (
	flagPrefix  = "tenant-settings.recording-rules."
	flagEnabled = flagPrefix + "enabled"
	flagAddress = flagPrefix + "client.address"
)

func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(
		&cfg.Enabled,
		flagEnabled,
		false,
		"Enable the storing of recording rules in tenant settings.",
	)
	fs.StringVar(
		&cfg.Address,
		flagAddress,
		"localhost:4040",
		"The address of the client for the tenant-settings",
	)
}

func (cfg *Config) Validate() error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Address == "" {
		return fmt.Errorf("%s is required", flagAddress)
	}
	return nil
}
