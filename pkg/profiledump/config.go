// Package profiledump provides bounded, best-effort native profile capture.
package profiledump

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Config holds process-wide bounds with provisional development defaults.
type Config struct {
	Cleaner                  CleanerConfig  `yaml:"cleaner"`
	Recorder                 RecorderConfig `yaml:"recorder"`
	MaxActivationWindow      time.Duration  `yaml:"max_activation_window"`
	DefaultCapturesPerSecond float64        `yaml:"default_captures_per_second"`
	MaxCapturesPerSecond     float64        `yaml:"max_captures_per_second"`
}

func DefaultConfig() Config {
	return Config{Cleaner: DefaultCleanerConfig(), Recorder: DefaultRecorderConfig(), MaxActivationWindow: time.Hour, DefaultCapturesPerSecond: 1, MaxCapturesPerSecond: 10}
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	d := DefaultConfig()
	c.Recorder.RegisterFlags(f)
	c.Cleaner.RegisterFlags(f)
	f.DurationVar(&c.MaxActivationWindow, "profile-dump.max-activation-window", d.MaxActivationWindow, "Maximum future activation window for a profile-debug-dump policy, measured at configuration load. Must be positive. Provisional development default.")
	f.Float64Var(&c.DefaultCapturesPerSecond, "profile-dump.default-captures-per-second", d.DefaultCapturesPerSecond, "Default profile capture rate per tenant per distributor when omitted from a policy. Must be positive and at most the global ceiling. This is not a fleet-wide quota. Provisional development default.")
	f.Float64Var(&c.MaxCapturesPerSecond, "profile-dump.max-captures-per-second", d.MaxCapturesPerSecond, "Hard ceiling for each tenant's local per-distributor profile capture rate. Must be finite and positive. This is not a fleet-wide quota. Provisional development default.")
}

func (c Config) Validate() error {
	if err := c.Cleaner.Validate(); err != nil {
		return err
	}
	if err := c.Recorder.Validate(); err != nil {
		return err
	}
	if c.MaxActivationWindow <= 0 {
		return fmt.Errorf("max_activation_window must be positive")
	}
	if !positiveFinite(c.MaxCapturesPerSecond) {
		return fmt.Errorf("max_captures_per_second must be finite and positive")
	}
	if !positiveFinite(c.DefaultCapturesPerSecond) || c.DefaultCapturesPerSecond > c.MaxCapturesPerSecond {
		return fmt.Errorf("default_captures_per_second must be finite, positive and at most %g", c.MaxCapturesPerSecond)
	}
	return nil
}

// TenantConfig is a nullable tenant policy. Pointers distinguish omitted values.
// Call Compile before publishing it.
type TenantConfig struct {
	ActiveUntil          *time.Time `yaml:"active_until" json:"active_until"`
	Selector             *string    `yaml:"selector,omitempty" json:"selector,omitempty"`
	Probability          *float64   `yaml:"probability" json:"probability"`
	MaxCapturesPerSecond *float64   `yaml:"max_captures_per_second,omitempty" json:"max_captures_per_second,omitempty"`
}

// Policy is an immutable, validated snapshot. Its zero value disables capture.
type Policy struct {
	activeUntil          time.Time
	selector             string
	matchers             []*labels.Matcher
	probability          float64
	maxCapturesPerSecond float64
	fingerprint          string
}

// Compile validates against now without mutating input or renewing the deadline.
func (c *TenantConfig) Compile(bounds Config, now time.Time) (Policy, error) {
	if err := bounds.Validate(); err != nil {
		return Policy{}, err
	}
	if c == nil {
		return Policy{}, nil
	}
	if c.ActiveUntil == nil {
		return Policy{}, fmt.Errorf("active_until is required")
	}
	if c.ActiveUntil.After(now.Add(bounds.MaxActivationWindow)) {
		return Policy{}, fmt.Errorf("active_until exceeds maximum activation window %s", bounds.MaxActivationWindow)
	}
	if c.Probability == nil {
		return Policy{}, fmt.Errorf("probability is required")
	}
	if !positiveFinite(*c.Probability) || *c.Probability > 1 {
		return Policy{}, fmt.Errorf("probability must be finite and in (0, 1]")
	}
	rate := bounds.DefaultCapturesPerSecond
	if c.MaxCapturesPerSecond != nil {
		rate = *c.MaxCapturesPerSecond
	}
	if !positiveFinite(rate) || rate > bounds.MaxCapturesPerSecond {
		return Policy{}, fmt.Errorf("max_captures_per_second must be finite, positive and at most %g", bounds.MaxCapturesPerSecond)
	}
	selector := "{}"
	if c.Selector != nil {
		selector = *c.Selector
	}
	matchers, err := model.ParseMetricSelector(selector)
	if err != nil {
		return Policy{}, fmt.Errorf("invalid selector: %w", err)
	}
	// Normalize matcher syntax and order for a stable fingerprint.
	parts := make([]string, len(matchers))
	for i, matcher := range matchers {
		parts[i] = matcher.String()
	}
	slices.Sort(parts)
	selector = "{" + strings.Join(slices.Compact(parts), ",") + "}"
	deadline := c.ActiveUntil.UTC()
	// Selector strings escape newlines, keeping these fields unambiguous.
	representation := fmt.Sprintf("v1\n%s\n%s\n%g\n%g", deadline.Format(time.RFC3339Nano), selector, *c.Probability, rate)
	return Policy{
		activeUntil: deadline, selector: selector, matchers: matchers,
		probability: *c.Probability, maxCapturesPerSecond: rate,
		fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(representation))),
	}, nil
}

func positiveFinite(v float64) bool { return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) }

func (p Policy) ActiveAt(now time.Time) bool   { return p.fingerprint != "" && now.Before(p.activeUntil) }
func (p Policy) ActiveUntil() time.Time        { return p.activeUntil }
func (p Policy) Selector() string              { return p.selector }
func (p Policy) Probability() float64          { return p.probability }
func (p Policy) MaxCapturesPerSecond() float64 { return p.maxCapturesPerSecond }
func (p Policy) Fingerprint() string           { return p.fingerprint }

// LabelLookup reads borrowed labels without mutating or retaining input.
// Get returns an empty string for absent labels.
type LabelLookup interface {
	Get(name string) string
}

// Matches reads stable borrowed labels, treating nil as empty.
// Callers must also check ActiveAt before admission.
func (p Policy) Matches(l LabelLookup) bool {
	if p.fingerprint == "" {
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
