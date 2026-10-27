package profiledump

import (
	"flag"
	"fmt"
	"time"
)

// CleanerConfig controls retention performed by the admin target.
type CleanerConfig struct {
	Retention time.Duration `yaml:"retention"`
}

func DefaultCleanerConfig() CleanerConfig {
	return CleanerConfig{Retention: 7 * 24 * time.Hour}
}

func (c *CleanerConfig) RegisterFlags(f *flag.FlagSet) {
	d := DefaultCleanerConfig()
	f.DurationVar(&c.Retention, "profile-dump.retention", d.Retention, "How long to retain captures before they become eligible for cleanup, which requires an admin target using the same bucket and storage prefix.")
}

func (c CleanerConfig) Validate() error {
	if c.Retention <= 0 {
		return fmt.Errorf("cleaner retention must be positive")
	}
	return nil
}
