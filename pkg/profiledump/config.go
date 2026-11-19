// Package profiledump provides best-effort capture of original profile bytes.
// Captures and uploads can be dropped or interrupted, leaving incomplete pairs.
package profiledump

import (
	"flag"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Config contains process-wide recorder and cleaner settings.
type Config struct {
	Cleaner  CleanerConfig  `yaml:",inline"`
	Recorder RecorderConfig `yaml:",inline"`
}

func DefaultConfig() Config {
	return Config{Cleaner: DefaultCleanerConfig(), Recorder: DefaultRecorderConfig()}
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	c.Recorder.RegisterFlags(f)
	c.Cleaner.RegisterFlags(f)
}

const maxActivationDuration = 24 * time.Hour

// TenantConfig enables capture when architecture.storage=v2. Configure it under
// overrides.<tenant>.profile_dump in the runtime configuration. Invalid policies
// reject the entire reload, preserving the previous configuration, and fail startup
// on initial load. The runtime-config manager retries failed candidates.
// ActiveUntil is required and sets when capture stops, at most 24 hours ahead of
// configuration validation.
// Probability is the required sampling probability in (0, 1], where 1 selects all
// matching profiles.
// Selector defaults to {} and matches incoming series labels.
// Each distributor admits capture attempts at 10 profiles per second with a burst
// of 10, shared across all enabled tenants. More distributors increase aggregate
// capacity. Sampling and resource limits can reduce the number of saved captures.
type TenantConfig struct {
	ActiveUntil *time.Time `yaml:"active_until" json:"active_until"`
	Selector    *string    `yaml:"selector,omitempty" json:"selector,omitempty"`
	Probability *float64   `yaml:"probability" json:"probability"`
}

// Policy is an immutable, validated snapshot. Its zero value disables capture.
// Configured policies have positive probability, including after expiry.
type Policy struct {
	activeUntil time.Time
	matchers    []*labels.Matcher
	probability float64
}

// Compile validates all fields, including expired policies. Absence disables capture.
func (c *TenantConfig) Compile(now time.Time) (Policy, error) {
	if c == nil {
		return Policy{}, nil
	}
	if c.ActiveUntil == nil {
		return Policy{}, fmt.Errorf("active_until is required")
	}
	if c.Probability == nil {
		return Policy{}, fmt.Errorf("probability is required")
	}
	if !(*c.Probability > 0 && *c.Probability <= 1) {
		return Policy{}, fmt.Errorf("probability must be in (0, 1]")
	}
	if c.ActiveUntil.After(now.Add(maxActivationDuration)) {
		return Policy{}, fmt.Errorf("active_until must not be more than 24 hours ahead")
	}
	selector := "{}"
	if c.Selector != nil {
		selector = *c.Selector
	}
	matchers, err := model.ParseMetricSelector(selector)
	if err != nil {
		return Policy{}, fmt.Errorf("invalid selector: %w", err)
	}
	return Policy{
		activeUntil: *c.ActiveUntil, matchers: matchers, probability: *c.Probability,
	}, nil
}

func (p Policy) ActiveAt(now time.Time) bool { return p.probability > 0 && now.Before(p.activeUntil) }
func (p Policy) ActiveUntil() time.Time      { return p.activeUntil }
func (p Policy) Probability() float64        { return p.probability }

// Matches evaluates only the compiled selector, regardless of policy activation.
func (p Policy) Matches(l model.Labels) bool {
	for _, matcher := range p.matchers {
		if !matcher.Matches(l.Get(matcher.Name)) {
			return false
		}
	}
	return true
}
