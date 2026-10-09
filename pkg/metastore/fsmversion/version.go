package fsmversion

import (
	"flag"
	"time"
)

type Version uint32

const (
	Unversioned Version = iota
	Baseline
)

const Latest = Baseline

type Config struct {
	CheckInterval   time.Duration `yaml:"check_interval" category:"advanced"`
	ActivationDelay time.Duration `yaml:"activation_delay" category:"advanced"`
	MaxVersion      uint          `yaml:"max_version" category:"advanced"`

	TestingSupportedVersion *Version `yaml:"-"`
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.CheckInterval, prefix+"check-interval", time.Minute, "How often the raft leader checks whether all metastore replicas support a newer FSM version and activates it. 0 to disable activation.")
	f.DurationVar(&c.ActivationDelay, prefix+"activation-delay", 0, "How long all metastore replicas must report support for a newer FSM version before the raft leader activates it. Replicas refuse to start with a binary that does not support the active FSM version, so the delay is the window in which a rollback remains possible.")
	f.UintVar(&c.MaxVersion, prefix+"max-version", 0, "Highest FSM version the raft leader activates. 0 means no limit.")
}

func (c *Config) Supported() Version {
	if c.TestingSupportedVersion != nil {
		return *c.TestingSupportedVersion
	}
	return Latest
}
