package profiledump

import (
	"flag"
	"fmt"
	"math"
	"time"
)

// RecorderConfig bounds one recorder with provisional development defaults.
type RecorderConfig struct {
	MaxObjectBytes           int64         `yaml:"max_object_bytes"`
	MaxRetainedBytes         int64         `yaml:"max_retained_bytes"`
	ProcessCapturesPerSecond float64       `yaml:"process_captures_per_second"`
	UploadTimeout            time.Duration `yaml:"upload_timeout"`
}

func DefaultRecorderConfig() RecorderConfig {
	return RecorderConfig{
		MaxObjectBytes: 16 << 20, MaxRetainedBytes: 64 << 20,
		ProcessCapturesPerSecond: 10,
		UploadTimeout:            10 * time.Second,
	}
}

func (c *RecorderConfig) RegisterFlags(f *flag.FlagSet) {
	d := DefaultRecorderConfig()
	f.Int64Var(&c.MaxObjectBytes, "profile-dump.max-object-bytes", d.MaxObjectBytes, "Maximum combined native payload and JSON sidecar bytes. Oversized captures are dropped.")
	f.Int64Var(&c.MaxRetainedBytes, "profile-dump.max-retained-bytes", d.MaxRetainedBytes, "Per-distributor budget for admitted capture buffers, including owned payload, JSON capacity, item overhead, queueing and uploads. Initial bounded metadata marshaling and provider allocations are outside this budget. This is not an RSS limit.")
	f.Float64Var(&c.ProcessCapturesPerSecond, "profile-dump.process-captures-per-second", d.ProcessCapturesPerSecond, "Aggregate capture admission rate per distributor. Tenant rates may exceed this rate, but admissions remain constrained by it. Omitted tenant rates default to min(1, this rate). Must be finite and positive. This is not a fleet quota.")
	f.DurationVar(&c.UploadTimeout, "profile-dump.upload-timeout", d.UploadTimeout, "One cooperative timeout covering both sequential uploads of a capture. Providers that ignore cancellation may exceed it.")
}

func (c RecorderConfig) Validate() error {
	if c.MaxObjectBytes <= 0 || c.MaxObjectBytes > int64(math.MaxInt)-itemReservation || c.MaxRetainedBytes < c.MaxObjectBytes+itemReservation {
		return fmt.Errorf("recorder capture size must fit an int and retained bytes must cover a maximum capture plus %d bytes item overhead", itemReservation)
	}
	if !positiveFinite(c.ProcessCapturesPerSecond) || c.UploadTimeout <= 0 {
		return fmt.Errorf("recorder rate and durations must be finite and positive")
	}
	return nil
}
