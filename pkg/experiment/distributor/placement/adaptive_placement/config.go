package adaptive_placement

import (
	"flag"
	"time"
)

const flagPrefix = "adaptive-sharding."

type Limits interface {
	ShardingLimits(tenant string) ShardingLimits
}

type ShardingLimits struct {
	TenantShards         uint64        `yaml:"adaptive_sharding_tenant_shards" json:"adaptive_sharding_tenant_shards"`
	DefaultDatasetShards uint64        `yaml:"adaptive_sharding_default_dataset_shards" json:"adaptive_sharding_default_dataset_shards"`
	LoadBalancing        LoadBalancing `yaml:"adaptive_sharding_load_balancing" json:"adaptive_sharding_load_balancing"`
	MinDatasetShards     uint64        `yaml:"adaptive_sharding_min_dataset_shards" json:"adaptive_sharding_min_dataset_shards"`
	MaxDatasetShards     uint64        `yaml:"adaptive_sharding_max_dataset_shards" json:"adaptive_sharding_max_dataset_shards"`
	UnitSizeBytes        uint64        `yaml:"adaptive_sharding_unit_size_bytes" json:"adaptive_sharding_unit_size_bytes"`
	BurstWindow          time.Duration `yaml:"adaptive_sharding_burst_window" json:"adaptive_sharding_burst_window"`
	DecayWindow          time.Duration `yaml:"adaptive_sharding_decay_window" json:"adaptive_sharding_decay_window"`
}

func (o *ShardingLimits) RegisterFlags(f *flag.FlagSet) {
	o.RegisterFlagsWithPrefix(flagPrefix, f)
}

func (o *ShardingLimits) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	o.LoadBalancing = DynamicLoadBalancing
	f.Var(&o.LoadBalancing, prefix+"load-balancing", "Load balancing strategy; "+validOptionsString+".")
	f.Uint64Var(&o.TenantShards, prefix+"tenant-shards", 0, "Number of shards per tenant. If 0, the limit is not applied.")
	f.Uint64Var(&o.DefaultDatasetShards, prefix+"default-dataset-shards", 1, "Default number of shards per dataset.")
	f.Uint64Var(&o.MinDatasetShards, prefix+"min-dataset-shards", 1, "Minimum number of shards per dataset.")
	f.Uint64Var(&o.MaxDatasetShards, prefix+"max-dataset-shards", 64, "Maximum number of shards per dataset.")
	f.Uint64Var(&o.UnitSizeBytes, prefix+"unit-size-bytes", 256<<10, "Size of the data chunk per shard in bytes")
	f.DurationVar(&o.BurstWindow, prefix+"burst-window", 17*time.Minute, "Burst window duration. During this period, subsequent scale-outs are intensified.")
	f.DurationVar(&o.DecayWindow, prefix+"decay-window", 19*time.Minute, "Decay window duration. During this period, scale-ins are postponed.")
}

func (o *ShardingLimits) Validate() error {
	// TODO.
	return nil
}

type Config struct {
	PlacementUpdateInterval  time.Duration `yaml:"placement_rules_update_interval" json:"placement_rules_update_interval"`
	PlacementRetentionPeriod time.Duration `yaml:"placement_rules_retention_period" json:"placement_rules_retention_period"`
	StatsConfidencePeriod    time.Duration `yaml:"stats_confidence_period" json:"stats_confidence_period"`
	StatsAggregationWindow   time.Duration `yaml:"stats_aggregation_window" json:"stats_aggregation_window"`
	StatsRetentionPeriod     time.Duration `yaml:"stats_retention_period" json:"stats_retention_period"`

	ExportShardLimitMetrics          bool `yaml:"export_shard_limit_metrics" json:"export_shard_limit_metrics"`
	ExportShardUsageMetrics          bool `yaml:"export_shard_usage_metrics" json:"export_shard_usage_metrics"`
	ExportShardUsageBreakdownMetrics bool `yaml:"export_shard_usage_breakdown_metrics" json:"export_shard_usage_breakdown_metrics"`
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	c.RegisterFlagsWithPrefix(flagPrefix, f)
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.PlacementUpdateInterval, prefix+"placement-rules-update-interval", 15*time.Second, "Interval between placement updates.")
	f.DurationVar(&c.PlacementRetentionPeriod, prefix+"placement-rules-retention-period", 15*time.Minute, "Retention period for placement rules that are not receiving any stats.")
	f.DurationVar(&c.StatsConfidencePeriod, prefix+"stats-confidence-period", 0, "Confidence period for stats. During this period, placement rules are not updated. If 0, placement rules may be built with incomplete stats.")
	f.DurationVar(&c.StatsAggregationWindow, prefix+"stats-aggregation-window", 3*time.Minute, "Aggregation window for stats.")
	f.DurationVar(&c.StatsRetentionPeriod, prefix+"stats-retention-period", 15*time.Minute, "Retention period for stats that are not receiving any updates.")
	f.BoolVar(&c.ExportShardLimitMetrics, prefix+"export-shard-limit-metrics", true, "Export shard limit metrics. If enabled, placement limits are exported as prometheus metrics.")
	f.BoolVar(&c.ExportShardUsageMetrics, prefix+"export-shard-usage-metrics", false, "Export shard usage metrics. If enabled, shard utilisation is exported as prometheus metrics.")
	f.BoolVar(&c.ExportShardUsageBreakdownMetrics, prefix+"export-shard-usage-breakdown-metrics", false, "Export shard usage breakdown metrics. If enabled, shard utilisation breakdown by shard and its owner is exported as prometheus metrics.")
}

func (c *Config) Validate() error {
	// TODO.
	return nil
}
