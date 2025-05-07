// This file is a modified copy of the usage groups implementation in Mimir:
//
// https://github.com/grafana/mimir/blob/0e8c09f237649e95dc1bf3f7547fd279c24bdcf9/pkg/ingester/activeseries/custom_trackers_config.go#L48

package validation

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

const (
	// Maximum number of usage groups that can be configured (per tenant).
	maxUsageGroups = 50

	// The usage group name to use when no user-defined usage groups matched.
	noMatchName = "other"
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
	usageGroupDiscardedBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "usage_group_discarded_bytes_total",
			Help:      "The total number of bytes that were discarded by usage group.",
		},
		[]string{"reason", "tenant", "usage_group"},
	)
)

type UsageGroupConfig struct {
	config map[string][]*labels.Matcher
}

func (c *UsageGroupConfig) GetUsageGroups(tenantID string, lbls phlaremodel.Labels) UsageGroupMatch {
	match := UsageGroupMatch{
		tenantID: tenantID,
	}

	for name, matchers := range c.config {
		if matchesAll(matchers, lbls) {
			match.names = append(match.names, name)
		}
	}

	return match
}

func (c *UsageGroupConfig) UnmarshalYAML(value *yaml.Node) error {
	m := make(map[string]string)
	err := value.DecodeWithOptions(&m, yaml.DecodeOptions{
		KnownFields: true,
	})
	if err != nil {
		return fmt.Errorf("malformed usage group config: %w", err)
	}

	*c, err = NewUsageGroupConfig(m)
	if err != nil {
		return err
	}
	return nil
}

func (c *UsageGroupConfig) UnmarshalJSON(bytes []byte) error {
	m := make(map[string]string)
	err := json.Unmarshal(bytes, &m)
	if err != nil {
		return fmt.Errorf("malformed usage group config: %w", err)
	}

	*c, err = NewUsageGroupConfig(m)
	if err != nil {
		return err
	}
	return nil
}

type UsageGroupMatch struct {
	tenantID string
	names    []string
}

func (m UsageGroupMatch) CountReceivedBytes(profileType string, n int64) {
	if len(m.names) == 0 {
		usageGroupReceivedDecompressedBytes.WithLabelValues(profileType, m.tenantID, noMatchName).Add(float64(n))
		return
	}

	for _, name := range m.names {
		usageGroupReceivedDecompressedBytes.WithLabelValues(profileType, m.tenantID, name).Add(float64(n))
	}
}

func (m UsageGroupMatch) CountDiscardedBytes(reason string, n int64) {
	if len(m.names) == 0 {
		usageGroupDiscardedBytes.WithLabelValues(reason, m.tenantID, noMatchName).Add(float64(n))
		return
	}

	for _, name := range m.names {
		usageGroupDiscardedBytes.WithLabelValues(reason, m.tenantID, name).Add(float64(n))
	}
}

func (m UsageGroupMatch) Names() []string {
	return m.names
}

func NewUsageGroupConfig(m map[string]string) (UsageGroupConfig, error) {
	if len(m) > maxUsageGroups {
		return UsageGroupConfig{}, fmt.Errorf("maximum number of usage groups is %d, got %d", maxUsageGroups, len(m))
	}

	config := UsageGroupConfig{
		config: make(map[string][]*labels.Matcher),
	}

	for name, matchersText := range m {
		if !utf8.ValidString(name) {
			return UsageGroupConfig{}, fmt.Errorf("usage group name %q is not valid UTF-8", name)
		}

		name = strings.TrimSpace(name)
		if name == "" {
			return UsageGroupConfig{}, fmt.Errorf("usage group name cannot be empty")
		}

		if name == noMatchName {
			return UsageGroupConfig{}, fmt.Errorf("usage group name %q is reserved", noMatchName)
		}

		matchers, err := parser.ParseMetricSelector(matchersText)
		if err != nil {
			return UsageGroupConfig{}, fmt.Errorf("failed to parse matchers for usage group %q: %w", name, err)
		}

		config.config[name] = matchers
	}

	return config, nil
}

func (o *Overrides) DistributorUsageGroups(tenantID string) *UsageGroupConfig {
	config := o.getOverridesForTenant(tenantID).DistributorUsageGroups

	// It should never be nil, but check just in case!
	if config == nil {
		config = &UsageGroupConfig{}
	}
	return config
}

func matchesAll(matchers []*labels.Matcher, lbls phlaremodel.Labels) bool {
	if len(lbls) == 0 {
		return false
	}

	for _, m := range matchers {
		matched := false
		for _, lbl := range lbls {
			if lbl.Name == m.Name {
				if !m.Matches(lbl.Value) {
					return false
				}
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
