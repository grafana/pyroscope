// Package profiledump defines the native profile capture format and runtime policy.
package profiledump

import (
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Config contains process-wide recorder and cleaner settings with provisional development defaults.
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

// TenantConfig is the nullable profile_debug_dump block in runtime overrides.
// Pointers distinguish omitted fields from explicit zero values.
type TenantConfig struct {
	ActiveUntil          *time.Time `yaml:"active_until" json:"active_until"`
	Selector             *string    `yaml:"selector,omitempty" json:"selector,omitempty"`
	Probability          *float64   `yaml:"probability" json:"probability"`
	MaxCapturesPerSecond *float64   `yaml:"max_captures_per_second,omitempty" json:"max_captures_per_second,omitempty"`
}

// Policy is an immutable, validated snapshot. Its zero value disables capture.
// Configured policies have positive probability, including after expiry.
type Policy struct {
	activeUntil          time.Time
	matchers             []*labels.Matcher
	probability          float64
	maxCapturesPerSecond float64
}

// Compile creates an immutable policy, defaulting an omitted tenant rate to
// min(1, processRate). Explicit tenant rates are independent of the process rate.
// It leaves the input and its absolute deadline unchanged.
func (c *TenantConfig) Compile(processRate float64) (Policy, error) {
	if !positiveFinite(processRate) {
		return Policy{}, fmt.Errorf("process_captures_per_second must be finite and positive")
	}
	if c == nil {
		return Policy{}, nil
	}
	if c.ActiveUntil == nil {
		return Policy{}, fmt.Errorf("active_until is required")
	}
	if c.Probability == nil {
		return Policy{}, fmt.Errorf("probability is required")
	}
	if !positiveFinite(*c.Probability) || *c.Probability > 1 {
		return Policy{}, fmt.Errorf("probability must be finite and in (0, 1]")
	}
	rate := min(1, processRate)
	if c.MaxCapturesPerSecond != nil {
		rate = *c.MaxCapturesPerSecond
	}
	if !positiveFinite(rate) {
		return Policy{}, fmt.Errorf("max_captures_per_second must be finite and positive")
	}
	selector := "{}"
	if c.Selector != nil {
		selector = *c.Selector
	}
	matchers, err := model.ParseMetricSelector(selector)
	if err != nil {
		return Policy{}, fmt.Errorf("invalid selector: %w", err)
	}
	deadline := c.ActiveUntil.UTC()
	return Policy{
		activeUntil: deadline, matchers: matchers,
		probability: *c.Probability, maxCapturesPerSecond: rate,
	}, nil
}

func positiveFinite(v float64) bool { return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) }

func (p Policy) ActiveAt(now time.Time) bool   { return p.probability > 0 && now.Before(p.activeUntil) }
func (p Policy) ActiveUntil() time.Time        { return p.activeUntil }
func (p Policy) Probability() float64          { return p.probability }
func (p Policy) MaxCapturesPerSecond() float64 { return p.maxCapturesPerSecond }

// LabelLookup reads borrowed labels. Get returns "" for an absent label.
type LabelLookup interface {
	Get(name string) string
}

// Matches evaluates the compiled selector without checking expiry.
// A nil lookup is an empty label set.
func (p Policy) Matches(l LabelLookup) bool {
	if p.probability == 0 {
		return false
	}
	for _, matcher := range p.matchers {
		value := ""
		if l != nil {
			value = l.Get(matcher.Name)
		}
		if !matcher.Matches(value) {
			return false
		}
	}
	return true
}
