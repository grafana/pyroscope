package profiledump

import (
	"flag"
	"fmt"
	"time"
)

// CleanerConfig contains provisional development defaults, not a production SLA.
type CleanerConfig struct {
	Retention time.Duration `yaml:"retention"`
}

func DefaultCleanerConfig() CleanerConfig {
	return CleanerConfig{Retention: 7 * 24 * time.Hour}
}

func (c *CleanerConfig) RegisterFlags(f *flag.FlagSet) {
	d := DefaultCleanerConfig()
	f.DurationVar(&c.Retention, "profile-dump.retention", d.Retention, "Capture retention measured from server ULID time. Cleanup runs hourly and deletes only fully expired hours. Deletion is eventual. Must be positive.")
}

func (c CleanerConfig) Validate() error {
	if c.Retention <= 0 {
		return fmt.Errorf("cleaner retention must be positive")
	}
	return nil
}
