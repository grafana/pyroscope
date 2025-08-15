package annotation

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/pyroscope/pkg/distributor/ingestlimits"
)

type ThrottledAnnotation struct {
	PeriodType        string `json:"periodType"`
	PeriodLimitMb     int    `json:"periodLimitMb"`
	LimitResetTime    int64  `json:"limitResetTime"`
	SamplingPeriodSec int    `json:"samplingPeriodSec"`
	SamplingRequests  int    `json:"samplingRequests"`
	UsageGroup        string `json:"usageGroup"`
}

func CreateTenantAnnotation(c *ingestlimits.Config) ([]byte, error) {
	a := &ProfileAnnotation{
		Body: ThrottledAnnotation{
			PeriodType:        c.PeriodType,
			PeriodLimitMb:     c.PeriodLimitMb,
			LimitResetTime:    c.LimitResetTime,
			SamplingPeriodSec: int(c.Sampling.Period.Seconds()),
			SamplingRequests:  c.Sampling.NumRequests,
		},
	}
	return json.Marshal(a)
}

func CreateUsageGroupAnnotation(c *ingestlimits.Config, usageGroup string) ([]byte, error) {
	l, ok := c.UsageGroups[usageGroup]
	if !ok {
		return nil, fmt.Errorf("usageGroup %s not found", usageGroup)
	}
	a := &ProfileAnnotation{
		Body: ThrottledAnnotation{
			PeriodType:        c.PeriodType,
			PeriodLimitMb:     l.PeriodLimitMb,
			LimitResetTime:    c.LimitResetTime,
			SamplingPeriodSec: int(c.Sampling.Period.Seconds()),
			SamplingRequests:  c.Sampling.NumRequests,
			UsageGroup:        usageGroup,
		},
	}
	return json.Marshal(a)
}
