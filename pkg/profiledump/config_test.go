package profiledump_test

import (
	"flag"
	"math"
	"strings"
	"testing"
	"time"

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
		wantErr bool
	}{
		{"defaults", func(*profiledump.TenantConfig) {}, false},
		{"missing deadline", func(c *profiledump.TenantConfig) { c.ActiveUntil = nil }, true},
		{"past", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(-time.Hour)) }, false},
		{"zero deadline", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(time.Time{}) }, false},
		{"now", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now) }, false},
		{"exact maximum", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(24 * time.Hour)) }, false},
		{"excessive", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.Add(24*time.Hour + time.Nanosecond)) }, true},
		{"deadline offset", func(c *profiledump.TenantConfig) {
			c.ActiveUntil = ptr(c.ActiveUntil.In(time.FixedZone("offset", 3600)))
		}, false},
		{"missing probability", func(c *profiledump.TenantConfig) { c.Probability = nil }, true},
		{"small probability", func(c *profiledump.TenantConfig) { c.Probability = ptr(math.SmallestNonzeroFloat64) }, false},
		{"zero probability", func(c *profiledump.TenantConfig) { c.Probability = ptr(0.0) }, true},
		{"negative probability", func(c *profiledump.TenantConfig) { c.Probability = ptr(-1.0) }, true},
		{"excessive probability", func(c *profiledump.TenantConfig) { c.Probability = ptr(1.01) }, true},
		{"NaN", func(c *profiledump.TenantConfig) { c.Probability = ptr(math.NaN()) }, true},
		{"positive infinity", func(c *profiledump.TenantConfig) { c.Probability = ptr(math.Inf(1)) }, true},
		{"negative infinity", func(c *profiledump.TenantConfig) { c.Probability = ptr(math.Inf(-1)) }, true},
		{"selector", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{service_name=~"checkout.*",env!="dev"}`) }, false},
		{"invalid selector", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{broken`) }, true},
		{"expired invalid", func(c *profiledump.TenantConfig) {
			c.ActiveUntil = ptr(now.Add(-time.Hour))
			c.Probability = ptr(0.0)
		}, true},
		{"expired invalid selector", func(c *profiledump.TenantConfig) {
			c.ActiveUntil = ptr(now.Add(-time.Hour))
			c.Selector = ptr(`{broken`)
		}, true},
		{"expired missing probability", func(c *profiledump.TenantConfig) {
			c.ActiveUntil = ptr(now.Add(-time.Hour))
			c.Probability = nil
		}, true},
		{"empty selector", func(c *profiledump.TenantConfig) { c.Selector = ptr("") }, true},
		{"invalid regex", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{service_name=~"["}`) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.change(c)
			p, err := c.Compile(now)
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, profiledump.Policy{}, p)
				return
			}
			require.NoError(t, err)
			require.Equal(t, *c.ActiveUntil, p.ActiveUntil())
			require.Equal(t, *c.Probability, p.Probability())
			require.Equal(t, now.Before(*c.ActiveUntil), p.ActiveAt(now))
		})
	}
	var disabled *profiledump.TenantConfig
	p, err := disabled.Compile(now)
	require.NoError(t, err)
	require.Equal(t, profiledump.Policy{}, p)
}

func TestPolicySnapshot(t *testing.T) {
	t.Parallel()
	c := validConfig()
	c.Selector = ptr(`{service_name=~"checkout.*",env!="dev"}`)
	p, err := c.Compile(now)
	require.NoError(t, err)
	deadline := *c.ActiveUntil
	require.True(t, p.ActiveAt(deadline.Add(-time.Nanosecond)))
	require.False(t, p.ActiveAt(deadline))
	require.False(t, p.ActiveAt(deadline.Add(time.Nanosecond)))
	// Input mutations cannot alter a compiled snapshot.
	*c.ActiveUntil = now.Add(time.Hour)
	*c.Probability = 0.5
	*c.Selector = "{}"
	require.Equal(t, deadline, p.ActiveUntil())
	require.Equal(t, 1.0, p.Probability())
	require.False(t, p.Matches(nil))
}

func TestPolicyMatches(t *testing.T) {
	t.Parallel()
	// External labels need not arrive sorted. Missing labels match an empty string.
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
		{`{ env!="dev", service_name =~ "checkout.*", env!="dev" }`, true, false},
		{`{service_name!~"checkout.*"}`, false, true},
		{`{missing=""}`, true, true},
		{`{missing!=""}`, false, false},
		{`{missing=~".*"}`, true, true},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			cfg := validConfig()
			cfg.Selector = &tc.selector
			policy, err := cfg.Compile(now)
			require.NoError(t, err)
			require.Equal(t, tc.populated, policy.Matches(borrowed))
			require.Equal(t, tc.empty, policy.Matches(nil))
		})
	}
}

func TestProfileDumpConfigurationSurface(t *testing.T) {
	t.Parallel()
	expected := profiledump.DefaultConfig()
	expected.Cleaner.Retention = 30 * 24 * time.Hour
	expected.Recorder = profiledump.RecorderConfig{MaxCaptureBytes: 1 << 20, MaxRetainedBytes: 2 << 20}
	var fromFlags profiledump.Config
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	fromFlags.RegisterFlags(flags)
	require.Equal(t, profiledump.DefaultConfig(), fromFlags)
	require.NoError(t, fromFlags.Recorder.Validate())
	require.NoError(t, fromFlags.Cleaner.Validate())
	require.NoError(t, flags.Parse([]string{"-profile-dump.max-capture-bytes=1048576", "-profile-dump.max-retained-bytes=2097152", "-profile-dump.retention=720h"}))
	require.Equal(t, expected, fromFlags)
	fromYAML := profiledump.DefaultConfig()
	decoder := yaml.NewDecoder(strings.NewReader(`max_capture_bytes: 1048576
max_retained_bytes: 2097152
retention: 720h
`))
	decoder.KnownFields(true)
	require.NoError(t, decoder.Decode(&fromYAML))
	require.Equal(t, expected, fromYAML)
	require.NoError(t, fromYAML.Recorder.Validate())
}

func TestRecorderConfig(t *testing.T) {
	for _, cfg := range []profiledump.RecorderConfig{
		{MaxCaptureBytes: 0, MaxRetainedBytes: 1024},
		{MaxCaptureBytes: 1024, MaxRetainedBytes: 1023},
		{MaxCaptureBytes: 1024, MaxRetainedBytes: 0},
		{MaxCaptureBytes: 1024, MaxRetainedBytes: -1},
	} {
		require.Error(t, cfg.Validate())
	}
}

func TestCleanerConfig(t *testing.T) {
	for _, retention := range []time.Duration{0, -time.Hour} {
		require.Error(t, (profiledump.CleanerConfig{Retention: retention}).Validate())
	}
}
