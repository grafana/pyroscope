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
	QueueCapacity            int           `yaml:"queue_capacity"`
	Workers                  int           `yaml:"workers"`
	TenantBurst              int           `yaml:"tenant_burst"`
	ProcessCapturesPerSecond float64       `yaml:"process_captures_per_second"`
	ProcessBurst             int           `yaml:"process_burst"`
	UploadTimeout            time.Duration `yaml:"upload_timeout"`
	ShutdownDrain            time.Duration `yaml:"shutdown_drain"`
	MaxTenantLimiters        int           `yaml:"max_tenant_limiters"`
	LimiterPruneInterval     time.Duration `yaml:"limiter_prune_interval"`
}

func DefaultRecorderConfig() RecorderConfig {
	return RecorderConfig{
		MaxObjectBytes: 16 << 20, MaxRetainedBytes: 64 << 20,
		QueueCapacity: 16, Workers: 2, TenantBurst: 1,
		ProcessCapturesPerSecond: 10, ProcessBurst: 2,
		UploadTimeout: 10 * time.Second, ShutdownDrain: 15 * time.Second,
		MaxTenantLimiters: 1024, LimiterPruneInterval: time.Minute,
	}
}

func (c *RecorderConfig) RegisterFlags(f *flag.FlagSet) {
	d := DefaultRecorderConfig()
	f.Int64Var(&c.MaxObjectBytes, "profile-dump.max-object-bytes", d.MaxObjectBytes, "Maximum complete capture envelope bytes. Oversized captures are dropped. Provisional development default.")
	f.Int64Var(&c.MaxRetainedBytes, "profile-dump.max-retained-bytes", d.MaxRetainedBytes, "Local recorder byte budget covering preparation, overlapping metadata and envelope buffers, queueing and uploads. Provisional development default.")
	f.IntVar(&c.QueueCapacity, "profile-dump.queue-capacity", d.QueueCapacity, "Maximum waiting captures per distributor. Enqueue never waits. Provisional development default.")
	f.IntVar(&c.Workers, "profile-dump.workers", d.Workers, "Fixed upload workers per distributor. Provisional development default.")
	f.IntVar(&c.TenantBurst, "profile-dump.tenant-burst", d.TenantBurst, "Object admission burst per tenant per distributor. Provisional development default.")
	f.Float64Var(&c.ProcessCapturesPerSecond, "profile-dump.process-captures-per-second", d.ProcessCapturesPerSecond, "Aggregate object admission rate per distributor, not a fleet quota. Provisional development default.")
	f.IntVar(&c.ProcessBurst, "profile-dump.process-burst", d.ProcessBurst, "Aggregate object admission burst per distributor. Provisional development default.")
	f.DurationVar(&c.UploadTimeout, "profile-dump.upload-timeout", d.UploadTimeout, "Timeout for each background capture upload. Provisional development default.")
	f.DurationVar(&c.ShutdownDrain, "profile-dump.shutdown-drain", d.ShutdownDrain, "Graceful drain interval before canceling remaining capture work. Provisional development default.")
	f.IntVar(&c.MaxTenantLimiters, "profile-dump.max-tenant-limiters", d.MaxTenantLimiters, "Maximum local tenant rate limiter entries. New tenants are dropped at capacity until inactive entries are pruned. Provisional development default.")
	f.DurationVar(&c.LimiterPruneInterval, "profile-dump.limiter-prune-interval", d.LimiterPruneInterval, "Interval for pruning removed or expired local tenant rate limiter entries, including without traffic. Provisional development default.")
}

func (c RecorderConfig) Validate() error {
	if c.MaxObjectBytes <= int64(HeaderSize) || c.MaxObjectBytes > int64(math.MaxInt)-(MaxMetadataSize+itemReservation) || c.MaxRetainedBytes < c.MaxObjectBytes+(MaxMetadataSize+itemReservation) {
		return fmt.Errorf("recorder object size must fit an int and retained bytes must cover a maximum object plus %d bytes encoding overhead", (MaxMetadataSize + itemReservation))
	}
	if c.QueueCapacity <= 0 || c.Workers <= 0 || c.TenantBurst <= 0 || c.ProcessBurst <= 0 || c.MaxTenantLimiters <= 0 {
		return fmt.Errorf("recorder queue, workers, bursts and tenant limiter capacity must be positive")
	}
	if !positiveFinite(c.ProcessCapturesPerSecond) || c.UploadTimeout <= 0 || c.ShutdownDrain <= 0 || c.LimiterPruneInterval <= 0 {
		return fmt.Errorf("recorder rate and durations must be finite and positive")
	}
	return nil
}
