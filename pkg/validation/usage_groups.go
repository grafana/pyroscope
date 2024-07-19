// This file is a modified copy of the usage groups implementation in Mimir:
//
// https://github.com/grafana/mimir/blob/0e8c09f237649e95dc1bf3f7547fd279c24bdcf9/pkg/ingester/activeseries/custom_trackers_config.go#L48

package validation

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Maximum number of usage groups that can be configured (per tenant).
	maxUsageGroups = 50
)

var (
	// This is a duplicate of distributor_received_decompressed_bytes, but with
	// usage_group as a label.
	usageGroupReceivedDecompressedBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "usage_group_received_decompressed_total",
			Help:      "The total number of decompressed bytes per profile received by usage group.",
		},
		[]string{"type", "tenant", "usage_group"},
	)

	// This is a duplicate of discarded_bytes_total, but with usage_group as a
	// label.
	UsageGroupDiscardedBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "usage_group_discarded_bytes_total",
			Help:      "The total number of bytes that were discarded by usage group.",
		},
		[]string{"reason", "tenant", "usage_group"},
	)
)

type UsageGroupConfig struct {
	config map[string]string
}

func NewUsageGroupConfig(m map[string]string) (UsageGroupConfig, error) {
	if len(m) > maxUsageGroups {
		return UsageGroupConfig{}, fmt.Errorf("maximum number of usage groups is %d, got %d", maxUsageGroups, len(m))
	}

	config := UsageGroupConfig{}
	if len(m) == 0 {
		return config, nil
	}

	config.config = make(map[string]string)
	for name, matchers := range m {
		config.config[name] = matchers
	}

	return config, nil
}
