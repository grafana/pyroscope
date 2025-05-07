package ingest_limits

import "time"

type Config struct {
	// PeriodType provides the limit period / interval (e.g., "hour"). Used in error messages only.
	PeriodType string `yaml:"period_type" json:"period_type"`
	// PeriodLimitMb provides the limit that is being set in MB. Used in error messages only.
	PeriodLimitMb int `yaml:"period_limit_mb" json:"period_limit_mb"`
	// LimitResetTime provides the time (Unix seconds) when the limit will reset. Used in error messages only.
	LimitResetTime int64 `yaml:"limit_reset_time" json:"limit_reset_time"`
	// LimitReached instructs distributors to allow or reject profiles.
	LimitReached bool `yaml:"limit_reached" json:"limit_reached"`
	// Sampling controls the sampling parameters when the limit is reached.
	Sampling SamplingConfig `yaml:"sampling" json:"sampling"`
	// UsageGroups controls ingestion for pre-configured usage groups.
	UsageGroups map[string]UsageGroup `yaml:"usage_groups" json:"usage_groups"`
}

// SamplingConfig describes the params of a simple probabilistic sampling mechanism.
//
// Distributors should allow up to NumRequests requests through and then apply a cooldown (Period) after which
// more requests can be let through.
type SamplingConfig struct {
	NumRequests int           `yaml:"num_requests" json:"num_requests"`
	Period      time.Duration `yaml:"period" json:"period"`
}

type UsageGroup struct {
	PeriodLimitMb int  `yaml:"period_limit_mb" json:"period_limit_mb"`
	LimitReached  bool `yaml:"limit_reached" json:"limit_reached"`
}
