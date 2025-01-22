package metrics

import (
	"github.com/prometheus/prometheus/model/labels"
)

type RecordingRule struct {
	profileType string
	metricName  string
	matchers    []*labels.Matcher
	keepLabels  []string
}

func RecordingRulesFromTenant(tenant string) []*RecordingRule {
	// TODO
	return []*RecordingRule{
		{
			profileType: "process_cpu:samples:count:cpu:nanoseconds",
			metricName:  "ride_sharing_app_car_cpu_nanoseconds",
			matchers: []*labels.Matcher{
				{
					Type:  labels.MatchEqual,
					Name:  "service_name",
					Value: "ride-sharing-app",
				},
				{
					Type:  labels.MatchEqual,
					Name:  "vehicle",
					Value: "car",
				},
			},
			keepLabels: []string{"region"},
		},
		{
			profileType: "process_cpu:samples:count:cpu:nanoseconds",
			metricName:  "ride_sharing_app_car_all_regions_cpu_nanoseconds",
			matchers: []*labels.Matcher{
				{
					Type:  labels.MatchEqual,
					Name:  "service_name",
					Value: "ride-sharing-app",
				},
				{
					Type:  labels.MatchEqual,
					Name:  "vehicle",
					Value: "car",
				},
			},
			keepLabels: []string{},
		},
	}
}
