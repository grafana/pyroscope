package ingest_limits

import (
	"encoding/json"
	"fmt"
)

func CreateTenantAnnotation(c *Config) ([]byte, error) {
	annotation := &ProfileAnnotation{
		Body: ThrottledAnnotation{
			PeriodType:        c.PeriodType,
			PeriodLimitMb:     c.PeriodLimitMb,
			LimitResetTime:    c.LimitResetTime,
			SamplingPeriodSec: int(c.Sampling.Period.Seconds()),
			SamplingRequests:  c.Sampling.NumRequests,
		},
	}
	return json.Marshal(annotation)
}

func CreateUsageGroupAnnotation(c *Config, usageGroup string) ([]byte, error) {
	l, ok := c.UsageGroups[usageGroup]
	if !ok {
		return nil, fmt.Errorf("usageGroup %s not found", usageGroup)
	}
	annotation := &ProfileAnnotation{
		Body: ThrottledAnnotation{
			PeriodType:        c.PeriodType,
			PeriodLimitMb:     l.PeriodLimitMb,
			LimitResetTime:    c.LimitResetTime,
			SamplingPeriodSec: int(c.Sampling.Period.Seconds()),
			SamplingRequests:  c.Sampling.NumRequests,
			UsageGroup:        usageGroup,
		},
	}
	return json.Marshal(annotation)
}

type ProfileAnnotation struct {
	Body interface{} `json:"body"`
}

const (
	ProfileAnnotationKeyThrottled = "pyroscope.ingest.throttled"
)

type ThrottledAnnotation struct {
	PeriodType        string `json:"periodType"`
	PeriodLimitMb     int    `json:"periodLimitMb"`
	LimitResetTime    int64  `json:"limitResetTime"`
	SamplingPeriodSec int    `json:"samplingPeriodSec"`
	SamplingRequests  int    `yaml:"samplingRequests"`
	UsageGroup        string `json:"usageGroup"`
}
