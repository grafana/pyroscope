// This file is a modified copy of the usage groups implementation in Mimir:
//
// https://github.com/grafana/mimir/blob/0e8c09f237649e95dc1bf3f7547fd279c24bdcf9/pkg/ingester/activeseries/custom_trackers_config.go#L48

package validation

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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

// templatePart represents a part of a parsed usage group name template
type templatePart struct {
	isLiteral bool
	value     string // literal text or label name for placeholder
}

// usageGroupEntry represents a single usage group configuration
type usageGroupEntry struct {
	matchers []*labels.Matcher
	// For static names, template is nil and name is used
	name string
	// For dynamic names, template contains the parsed template parts
	template []templatePart
}

type UsageGroupConfig struct {
	config map[string][]*labels.Matcher

	parsedEntries []usageGroupEntry
}

const dynamicLabelNamePrefix = "${labels."

type UsageGroupEvaluator struct {
	logger log.Logger
}

func NewUsageGroupEvaluator(logger log.Logger) *UsageGroupEvaluator {
	return &UsageGroupEvaluator{
		logger: logger,
	}
}

func (e *UsageGroupEvaluator) GetMatch(tenantID string, c *UsageGroupConfig, lbls phlaremodel.Labels) UsageGroupMatch {
	match := UsageGroupMatch{
		tenantID: tenantID,
		names:    make([]UsageGroupMatchName, 0, len(c.parsedEntries)),
	}

	for _, entry := range c.parsedEntries {
		if c.matchesAll(entry.matchers, lbls) {
			if entry.template != nil {
				resolvedName, err := c.expandTemplate(entry.template, lbls)
				if err != nil {
					level.Warn(e.logger).Log(
						"msg", "failed to expand usage group template, skipping usage group",
						"err", err,
						"usage_group", entry.name)
					continue
				}
				if resolvedName == "" {
					level.Warn(e.logger).Log(
						"msg", "usage group template expanded to empty string, skipping usage group",
						"usage_group", entry.name)
					continue
				}
				match.names = append(match.names, UsageGroupMatchName{
					ConfiguredName: entry.name,
					ResolvedName:   resolvedName,
				})
			} else {
				match.names = append(match.names, UsageGroupMatchName{
					ConfiguredName: entry.name,
					ResolvedName:   entry.name,
				})
			}
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

	entries, rawData, err := parseUsageGroupEntries(m)
	if err != nil {
		return err
	}
	c.parsedEntries = entries
	c.config = rawData
	return nil
}

func (c *UsageGroupConfig) UnmarshalJSON(bytes []byte) error {
	m := make(map[string]string)
	err := json.Unmarshal(bytes, &m)
	if err != nil {
		return fmt.Errorf("malformed usage group config: %w", err)
	}

	entries, rawData, err := parseUsageGroupEntries(m)
	if err != nil {
		return err
	}
	c.parsedEntries = entries
	c.config = rawData
	return nil
}

type UsageGroupMatch struct {
	tenantID string
	names    []UsageGroupMatchName
}

type UsageGroupMatchName struct {
	ConfiguredName string
	ResolvedName   string
}

func (m *UsageGroupMatchName) IsMoreSpecificThan(other *UsageGroupMatchName) bool {
	return !strings.Contains(m.ConfiguredName, dynamicLabelNamePrefix) && strings.Contains(other.ConfiguredName, dynamicLabelNamePrefix)
}

func (m *UsageGroupMatchName) String() string {
	return fmt.Sprintf("{configured: %s, resolved: %s}", m.ConfiguredName, m.ResolvedName)
}

func (m UsageGroupMatch) CountReceivedBytes(profileType string, n int64) {
	if len(m.names) == 0 {
		usageGroupReceivedDecompressedBytes.WithLabelValues(profileType, m.tenantID, noMatchName).Add(float64(n))
		return
	}

	for _, name := range m.names {
		usageGroupReceivedDecompressedBytes.WithLabelValues(profileType, m.tenantID, name.ResolvedName).Add(float64(n))
	}
}

func (m UsageGroupMatch) CountDiscardedBytes(reason string, n int64) {
	if len(m.names) == 0 {
		usageGroupDiscardedBytes.WithLabelValues(reason, m.tenantID, noMatchName).Add(float64(n))
		return
	}

	for _, name := range m.names {
		usageGroupDiscardedBytes.WithLabelValues(reason, m.tenantID, name.ResolvedName).Add(float64(n))
	}
}

func (m UsageGroupMatch) Names() []UsageGroupMatchName {
	return m.names
}

func NewUsageGroupConfig(m map[string]string) (*UsageGroupConfig, error) {
	entries, rawData, err := parseUsageGroupEntries(m)
	if err != nil {
		return nil, err
	}
	config := &UsageGroupConfig{
		parsedEntries: entries,
		config:        rawData,
	}
	return config, nil
}

func parseUsageGroupEntries(m map[string]string) ([]usageGroupEntry, map[string][]*labels.Matcher, error) {
	if len(m) > maxUsageGroups {
		return nil, nil, fmt.Errorf("maximum number of usage groups is %d, got %d", maxUsageGroups, len(m))
	}

	rawData := make(map[string][]*labels.Matcher)
	entries := make([]usageGroupEntry, 0, len(m))

	for name, matchersText := range m {
		if !utf8.ValidString(name) {
			return nil, nil, fmt.Errorf("usage group name %q is not valid UTF-8", name)
		}

		name = strings.TrimSpace(name)
		if name == "" {
			return nil, nil, fmt.Errorf("usage group name cannot be empty")
		}

		if name == noMatchName {
			return nil, nil, fmt.Errorf("usage group name %q is reserved", noMatchName)
		}

		matchers, err := parser.ParseMetricSelector(matchersText)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse matchers for usage group %q: %w", name, err)
		}

		entry := usageGroupEntry{
			matchers: matchers,
			name:     name,
		}

		if strings.Contains(name, dynamicLabelNamePrefix) {
			template, err := parseTemplate(name)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse template for usage group %q: %w", name, err)
			}
			entry.template = template
		}

		entries = append(entries, entry)
		rawData[name] = matchers
	}

	return entries, rawData, nil
}

// parseTemplate parses a usage group name template into parts
func parseTemplate(name string) ([]templatePart, error) {
	var parts []templatePart
	remaining := name

	for len(remaining) > 0 {
		before, after, found := strings.Cut(remaining, dynamicLabelNamePrefix)

		// add literal part before placeholder (if any)
		if len(before) > 0 {
			parts = append(parts, templatePart{
				isLiteral: true,
				value:     before,
			})
		}

		if !found {
			break
		}

		labelName, afterBrace, foundBrace := strings.Cut(after, "}")
		if !foundBrace {
			return nil, fmt.Errorf("unclosed placeholder")
		}

		if labelName == "" {
			return nil, fmt.Errorf("empty label name in placeholder")
		}

		parts = append(parts, templatePart{
			isLiteral: false,
			value:     labelName,
		})

		remaining = afterBrace
	}

	return parts, nil
}

func (o *Overrides) DistributorUsageGroups(tenantID string) *UsageGroupConfig {
	config := o.getOverridesForTenant(tenantID).DistributorUsageGroups

	// It should never be nil, but check just in case!
	if config == nil {
		config = &UsageGroupConfig{}
	}
	return config
}

func (c *UsageGroupConfig) matchesAll(matchers []*labels.Matcher, lbls phlaremodel.Labels) bool {
	if len(lbls) == 0 && len(matchers) > 0 {
		return false
	}

	for _, m := range matchers {
		if lbl, ok := lbls.GetLabel(m.Name); ok {
			if !m.Matches(lbl.Value) {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func (c *UsageGroupConfig) expandTemplate(template []templatePart, lbls phlaremodel.Labels) (string, error) {
	var result strings.Builder
	result.Grow(len(template) * 8)

	for _, part := range template {
		if part.isLiteral {
			result.WriteString(part.value)
		} else {
			value, found := lbls.GetLabel(part.value)
			if !found {
				return "", fmt.Errorf("label %q not found", part.value)
			}
			if value.Value == "" {
				return "", fmt.Errorf("label %q is empty", part.value)
			}
			result.WriteString(value.Value)
		}
	}

	return result.String(), nil
}
