package ingest_limits

import "time"

type Config struct {
	PeriodType     string         `yaml:"period_type" json:"period_type" doc:"hidden"`
	PeriodLimitMb  int            `yaml:"period_limit_mb" json:"period_limit_mb" doc:"hidden"`
	LimitResetTime int64          `yaml:"limit_reset_time" json:"limit_reset_time" doc:"hidden"`
	LimitReached   bool           `yaml:"limit_reached" json:"limit_reached" doc:"hidden"`
	Sampling       SamplingConfig `yaml:"sampling" json:"sampling" doc:"hidden"`
}

type SamplingConfig struct {
	NumRequests int           `yaml:"num_requests" json:"num_requests" doc:"hidden"`
	Period      time.Duration `yaml:"period" json:"period" doc:"hidden"`
}
