package profiledump

import (
	"flag"
	"fmt"
	"time"
)

// CleanerConfig contains provisional development defaults, not a production SLA.
type CleanerConfig struct {
	Retention     time.Duration `yaml:"retention"`
	SweepInterval time.Duration `yaml:"sweep_interval"`
	SweepTimeout  time.Duration `yaml:"sweep_timeout"`
	MaxEntries    int           `yaml:"max_entries"`
}

func DefaultCleanerConfig() CleanerConfig {
	return CleanerConfig{Retention: 7 * 24 * time.Hour, SweepInterval: time.Hour, SweepTimeout: time.Minute, MaxEntries: 10000}
}

func (c *CleanerConfig) RegisterFlags(f *flag.FlagSet) {
	d := DefaultCleanerConfig()
	f.DurationVar(&c.Retention, "profile-dump.retention", d.Retention, "Capture retention measured from server ULID time. Deletion is eventual. Must be positive. Provisional development default.")
	f.DurationVar(&c.SweepInterval, "profile-dump.sweep-interval", d.SweepInterval, "Delay between admin capture cleanup passes. Must be positive. Provisional development default.")
	f.DurationVar(&c.SweepTimeout, "profile-dump.sweep-timeout", d.SweepTimeout, "Cooperative time budget per admin capture cleanup pass and wait for a listing to produce its next entry. Providers may exceed it. Paused listings resume on later passes. Must be positive. Provisional development default.")
	f.IntVar(&c.MaxEntries, "profile-dump.cleanup-max-entries", d.MaxEntries, "Maximum listing entries processed per cleanup pass, excluding provider prefetch and cancellation draining. Listings pause at the budget and resume without replay. Must be positive. Provisional development default.")
}

func (c CleanerConfig) Validate() error {
	if c.Retention <= 0 || c.SweepInterval <= 0 || c.SweepTimeout <= 0 || c.MaxEntries <= 0 {
		return fmt.Errorf("cleaner retention, sweep interval, sweep timeout and max entries must be positive")
	}
	return nil
}
