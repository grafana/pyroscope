package quota

import "time"

type Config struct {
	PeriodType      string         `yaml:"period_type" json:"period_type" doc:"hidden"`
	PeriodLimitMb   int            `yaml:"period_limit_mb" json:"period_limit_mb" doc:"hidden"`
	NextPeriodStart int64          `yaml:"next_period_start" json:"next_period_start" doc:"hidden"`
	QuotaReached    bool           `yaml:"quota_reached" json:"quota_reached" doc:"hidden"`
	QuotaSampling   SamplingConfig `yaml:"quota_sampling" json:"quota_sampling" doc:"hidden"`
}

type SamplingConfig struct {
	NumRequests int           `yaml:"num_requests" json:"num_requests" doc:"hidden"`
	Period      time.Duration `yaml:"period" json:"period" doc:"hidden"`
}
