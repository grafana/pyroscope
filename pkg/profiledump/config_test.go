package profiledump_test

import (
	"encoding/json"
	"flag"
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

var now = time.Date(2026, 9, 16, 12, 0, 0, 123456789, time.UTC)

func ptr[T any](v T) *T { return &v }

func validConfig() *profiledump.TenantConfig {
	return &profiledump.TenantConfig{ActiveUntil: ptr(now.Add(time.Minute)), Probability: ptr(1.0)}
}

func TestCompile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		change  func(*profiledump.TenantConfig)
		wantErr string
	}{
		{"defaults", func(*profiledump.TenantConfig) {}, ""},
		{"missing deadline", func(c *profiledump.TenantConfig) { c.ActiveUntil = nil }, "active_until is required"},
		{"past", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(-time.Hour)) }, ""},
		{"window boundary", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(time.Hour)) }, ""},
		{"over window", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(time.Hour + time.Nanosecond)) }, "maximum activation window"},
		{"missing probability", func(c *profiledump.TenantConfig) { c.Probability = nil }, "probability is required"},
		{"small probability", func(c *profiledump.TenantConfig) { c.Probability = ptr(math.SmallestNonzeroFloat64) }, ""},
		{"selector", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{service_name=~"checkout.*",env!="dev"}`) }, ""},
		{"invalid selector", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{broken`) }, "invalid selector"},
		{"empty selector", func(c *profiledump.TenantConfig) { c.Selector = ptr("") }, "invalid selector"},
		{"invalid regex", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{service_name=~"["}`) }, "invalid selector"},
		{"rate ceiling", func(c *profiledump.TenantConfig) { c.MaxCapturesPerSecond = ptr(10.0) }, ""},
		{"fractional rate", func(c *profiledump.TenantConfig) { c.MaxCapturesPerSecond = ptr(0.5) }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.change(c)
			p, err := c.Compile(profiledump.DefaultConfig(), now)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.ActiveUntil.UTC(), p.ActiveUntil())
			require.Equal(t, *c.Probability, p.Probability())
			require.Equal(t, now.Before(*c.ActiveUntil), p.ActiveAt(now))
			if c.Selector == nil {
				require.Equal(t, "{}", p.Selector())
				require.True(t, p.Matches(labels.EmptyLabels()))
			}
			if c.MaxCapturesPerSecond == nil {
				require.Equal(t, 1.0, p.MaxCapturesPerSecond())
			}
		})
	}
	for _, v := range []float64{0, -1, 1.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := validConfig()
		c.Probability = ptr(v)
		_, err := c.Compile(profiledump.DefaultConfig(), now)
		require.ErrorContains(t, err, "probability")
	}
	for _, v := range []float64{0, -1, 10.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := validConfig()
		c.MaxCapturesPerSecond = ptr(v)
		_, err := c.Compile(profiledump.DefaultConfig(), now)
		require.ErrorContains(t, err, "max_captures_per_second")
	}
	var disabled *profiledump.TenantConfig
	p, err := disabled.Compile(profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	require.False(t, p.ActiveAt(now))
	require.False(t, p.Matches(labels.EmptyLabels()))
}

func TestPolicySnapshot(t *testing.T) {
	t.Parallel()
	c := validConfig()
	c.Selector = ptr(`{service_name=~"checkout.*",env!="dev"}`)
	p, err := c.Compile(profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	deadline := *c.ActiveUntil
	require.True(t, p.ActiveAt(deadline.Add(-time.Nanosecond)))
	require.False(t, p.ActiveAt(deadline))
	require.False(t, p.ActiveAt(deadline.Add(time.Nanosecond)))
	require.True(t, p.Matches(labels.FromStrings("service_name", "checkout-api", "env", "prod")))
	require.False(t, p.Matches(labels.FromStrings("service_name", "checkout-api", "env", "dev")))
	require.False(t, p.Matches(labels.EmptyLabels()))
	// Input mutations and overwriting a returned value cannot alter a snapshot.
	*c.ActiveUntil = now.Add(time.Hour)
	*c.Probability = 0.5
	*c.Selector = "{}"
	require.Equal(t, deadline, p.ActiveUntil())
	require.Equal(t, 1.0, p.Probability())
	require.False(t, p.Matches(labels.EmptyLabels()))
	copyOfPolicy := p
	p = profiledump.Policy{}
	require.False(t, p.ActiveAt(now))
	require.True(t, copyOfPolicy.ActiveAt(now))
}

func TestPolicyBorrowedLabelLookup(t *testing.T) {
	t.Parallel()
	// Unsorted and missing labels must retain Prometheus matching semantics.
	borrowed := model.Labels{
		{Name: "service_name", Value: "checkout-api"},
		{Name: "env", Value: "prod"},
	}
	for _, tc := range []struct {
		selector         string
		populated, empty bool
	}{
		{"{}", true, true},
		{`{service_name="checkout-api"}`, true, false},
		{`{service_name!="checkout-api"}`, false, true},
		{`{service_name=~"checkout.*",env!="dev"}`, true, false},
		{`{service_name!~"checkout.*"}`, false, true},
		{`{missing=""}`, true, true},
		{`{missing!=""}`, false, false},
		{`{missing=~".*"}`, true, true},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			cfg := validConfig()
			cfg.Selector = &tc.selector
			policy, err := cfg.Compile(profiledump.DefaultConfig(), now)
			require.NoError(t, err)
			require.Equal(t, tc.populated, policy.Matches(borrowed))
			require.Equal(t, policy.Matches(borrowed.ToPrometheusLabels()), policy.Matches(borrowed))
			require.Equal(t, tc.empty, policy.Matches(nil))
			require.Equal(t, policy.Matches(labels.EmptyLabels()), policy.Matches(nil))
		})
	}
}

func TestPolicyFingerprintAndRoundTrip(t *testing.T) {
	t.Parallel()
	c := validConfig()
	omitted, err := c.Compile(profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	c.Selector = ptr("{}")
	explicit, err := c.Compile(profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	require.Equal(t, omitted.Fingerprint(), explicit.Fingerprint())
	c.Selector = ptr(`{service_name="checkout",env!="dev"}`)
	original, err := c.Compile(profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	for _, format := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{{"yaml", yaml.Marshal, yaml.Unmarshal}, {"json", json.Marshal, json.Unmarshal}} {
		t.Run(format.name, func(t *testing.T) {
			b, err := format.marshal(c)
			require.NoError(t, err)
			var decoded profiledump.TenantConfig
			require.NoError(t, format.unmarshal(b, &decoded))
			require.Equal(t, c, &decoded)
			// A later reload/restart, even after expiry, preserves the deadline and fingerprint.
			for _, at := range []time.Time{now.Add(30 * time.Second), now.Add(time.Hour)} {
				p, err := decoded.Compile(profiledump.DefaultConfig(), at)
				require.NoError(t, err)
				require.Equal(t, original.Fingerprint(), p.Fingerprint())
				require.Equal(t, original.ActiveUntil(), p.ActiveUntil())
			}
		})
	}
	c.Selector = ptr(`{ env!="dev", service_name = "checkout",env!="dev" }`)
	c.MaxCapturesPerSecond = ptr(1.0)
	c.ActiveUntil = ptr(c.ActiveUntil.In(time.FixedZone("offset", 3600)))
	equivalent, err := c.Compile(profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	require.Equal(t, original.Fingerprint(), equivalent.Fingerprint())
	for _, change := range []func(*profiledump.TenantConfig){
		func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(2 * time.Minute)) },
		func(c *profiledump.TenantConfig) { c.Probability = ptr(0.5) },
		func(c *profiledump.TenantConfig) { c.MaxCapturesPerSecond = ptr(2.0) },
		func(c *profiledump.TenantConfig) { c.Selector = ptr("{}") },
	} {
		changed := *c
		change(&changed)
		p, err := changed.Compile(profiledump.DefaultConfig(), now)
		require.NoError(t, err)
		require.NotEqual(t, original.Fingerprint(), p.Fingerprint())
	}
}

func TestGlobalConfig(t *testing.T) {
	t.Parallel()
	var defaults profiledump.Config
	defaults.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	require.Equal(t, profiledump.DefaultConfig(), defaults)
	require.NoError(t, defaults.Validate())
	for _, duration := range []time.Duration{0, -time.Second} {
		c := defaults
		c.MaxActivationWindow = duration
		require.Error(t, c.Validate())
	}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := defaults
		c.MaxCapturesPerSecond = v
		require.Error(t, c.Validate())
		c = defaults
		c.DefaultCapturesPerSecond = v
		require.Error(t, c.Validate())
	}
	defaults.DefaultCapturesPerSecond = 11
	require.Error(t, defaults.Validate())
	bounds := profiledump.Config{Cleaner: profiledump.DefaultCleanerConfig(), Recorder: profiledump.DefaultRecorderConfig(), MaxActivationWindow: 2 * time.Hour, DefaultCapturesPerSecond: 20, MaxCapturesPerSecond: 30}
	c := validConfig()
	c.ActiveUntil = ptr(now.Add(90 * time.Minute))
	p, err := c.Compile(bounds, now)
	require.NoError(t, err)
	require.Equal(t, 20.0, p.MaxCapturesPerSecond())
}
