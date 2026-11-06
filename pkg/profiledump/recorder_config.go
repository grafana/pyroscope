package profiledump

import (
	"flag"
	"fmt"
)

// RecorderConfig bounds capture work in one distributor.
type RecorderConfig struct {
	MaxCaptureBytes  int64 `yaml:"max_capture_bytes"`
	MaxRetainedBytes int64 `yaml:"max_retained_bytes"`
}

func DefaultRecorderConfig() RecorderConfig {
	return RecorderConfig{
		MaxCaptureBytes:  16 << 20,
		MaxRetainedBytes: 64 << 20,
	}
}

func (c *RecorderConfig) RegisterFlags(f *flag.FlagSet) {
	d := DefaultRecorderConfig()
	f.Int64Var(&c.MaxCaptureBytes, "profile-dump.max-capture-bytes", d.MaxCaptureBytes, "Maximum capture size in bytes of the combined profile and JSON metadata. Larger captures are dropped.")
	f.Int64Var(&c.MaxRetainedBytes, "profile-dump.max-retained-bytes", d.MaxRetainedBytes, "Total profile and metadata bytes held by queued and uploading captures per distributor. New captures are dropped when there is insufficient space.")
}

func (c RecorderConfig) Validate() error {
	if c.MaxCaptureBytes <= 0 || c.MaxRetainedBytes <= 0 || c.MaxRetainedBytes < c.MaxCaptureBytes {
		return fmt.Errorf("recorder capture and retained bytes must be positive and retained bytes must cover a maximum capture")
	}
	return nil
}
