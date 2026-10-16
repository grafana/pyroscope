package profiledump_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"strings"
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
		{"zero deadline", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(time.Time{}) }, ""},
		{"deadline offset", func(c *profiledump.TenantConfig) {
			c.ActiveUntil = ptr(c.ActiveUntil.In(time.FixedZone("offset", 3600)))
		}, ""},
		{"far future", func(c *profiledump.TenantConfig) { c.ActiveUntil = ptr(now.AddDate(10, 0, 0)) }, ""},
		{"missing probability", func(c *profiledump.TenantConfig) { c.Probability = nil }, "probability is required"},
		{"small probability", func(c *profiledump.TenantConfig) { c.Probability = ptr(math.SmallestNonzeroFloat64) }, ""},
		{"selector", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{service_name=~"checkout.*",env!="dev"}`) }, ""},
		{"invalid selector", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{broken`) }, "invalid selector"},
		{"empty selector", func(c *profiledump.TenantConfig) { c.Selector = ptr("") }, "invalid selector"},
		{"invalid regex", func(c *profiledump.TenantConfig) { c.Selector = ptr(`{service_name=~"["}`) }, "invalid selector"},
		{"process rate", func(c *profiledump.TenantConfig) { c.MaxCapturesPerSecond = ptr(10.0) }, ""},
		{"above process rate", func(c *profiledump.TenantConfig) { c.MaxCapturesPerSecond = ptr(100.0) }, ""},
		{"fractional rate", func(c *profiledump.TenantConfig) { c.MaxCapturesPerSecond = ptr(0.5) }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.change(c)
			p, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.ActiveUntil.UTC(), p.ActiveUntil())
			require.Equal(t, *c.Probability, p.Probability())
			require.Equal(t, now.Before(*c.ActiveUntil), p.ActiveAt(now))
			if c.Selector == nil {
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
		_, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
		require.ErrorContains(t, err, "probability")
	}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := validConfig()
		c.MaxCapturesPerSecond = ptr(v)
		_, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
		require.ErrorContains(t, err, "max_captures_per_second")
	}
	var disabled *profiledump.TenantConfig
	p, err := disabled.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
	require.NoError(t, err)
	require.False(t, p.ActiveAt(now))
	require.False(t, p.Matches(labels.EmptyLabels()))
}

func TestPolicySnapshot(t *testing.T) {
	t.Parallel()
	c := validConfig()
	c.Selector = ptr(`{service_name=~"checkout.*",env!="dev"}`)
	p, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
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
	// External labels need not arrive sorted. Missing values must retain the
	// same empty-string semantics as the existing Prometheus representation.
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
			policy, err := cfg.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
			require.NoError(t, err)
			require.Equal(t, tc.populated, policy.Matches(borrowed))
			require.Equal(t, policy.Matches(borrowed.ToPrometheusLabels()), policy.Matches(borrowed))
			require.Equal(t, tc.empty, policy.Matches(nil))
			require.Equal(t, policy.Matches(labels.EmptyLabels()), policy.Matches(nil))
		})
	}
}

func TestPolicyRoundTrip(t *testing.T) {
	t.Parallel()
	c := validConfig()
	omitted, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
	require.NoError(t, err)
	c.Selector = ptr("{}")
	explicit, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
	require.NoError(t, err)
	for _, policy := range []profiledump.Policy{omitted, explicit} {
		require.True(t, policy.Matches(labels.EmptyLabels()))
		require.True(t, policy.Matches(labels.FromStrings("service_name", "checkout")))
	}
	require.Equal(t, omitted.MaxCapturesPerSecond(), explicit.MaxCapturesPerSecond())
	c.Selector = ptr(`{service_name="checkout",env!="dev"}`)
	original, err := c.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
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
			// A later reload/restart, even after expiry, preserves the policy fields and selection.
			for _, at := range []time.Time{now.Add(30 * time.Second), now.Add(time.Hour)} {
				p, err := decoded.Compile(profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
				require.NoError(t, err)
				require.Equal(t, original.Probability(), p.Probability())
				require.Equal(t, original.MaxCapturesPerSecond(), p.MaxCapturesPerSecond())
				require.True(t, p.Matches(labels.FromStrings("service_name", "checkout", "env", "prod")))
				require.False(t, p.Matches(labels.FromStrings("service_name", "checkout", "env", "dev")))
				require.False(t, p.Matches(labels.FromStrings("service_name", "payments", "env", "prod")))
				require.Equal(t, at.Before(p.ActiveUntil()), p.ActiveAt(at))
				require.Equal(t, original.ActiveUntil(), p.ActiveUntil())
			}
		})
	}
}

func TestGlobalConfig(t *testing.T) {
	t.Parallel()
	var defaults profiledump.Config
	defaults.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	require.Equal(t, profiledump.DefaultConfig(), defaults)
	require.NoError(t, defaults.Recorder.Validate())
	require.NoError(t, defaults.Cleaner.Validate())
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := defaults
		c.Recorder.ProcessCapturesPerSecond = v
		require.Error(t, c.Recorder.Validate())
		_, err := validConfig().Compile(c.Recorder.ProcessCapturesPerSecond)
		require.Error(t, err)
	}
}

func TestDerivedTenantRates(t *testing.T) {
	t.Parallel()
	for _, processRate := range []float64{math.SmallestNonzeroFloat64, 0.5, math.Nextafter(1, 0), 1, math.Nextafter(1, 2), 10, math.MaxFloat64} {
		t.Run(fmt.Sprint(processRate), func(t *testing.T) {
			c := validConfig()
			omitted, err := c.Compile(processRate)
			require.NoError(t, err)
			require.Equal(t, min(1, processRate), omitted.MaxCapturesPerSecond())
			for _, tenantRate := range []float64{min(1, processRate), processRate, math.MaxFloat64} {
				c.MaxCapturesPerSecond = ptr(tenantRate)
				explicit, err := c.Compile(processRate)
				require.NoError(t, err)
				require.Equal(t, tenantRate, explicit.MaxCapturesPerSecond())
			}
		})
	}
}

func TestProfileDumpConfigurationSurface(t *testing.T) {
	t.Parallel()
	expected := profiledump.DefaultConfig()
	expected.Recorder = profiledump.RecorderConfig{MaxObjectBytes: 1 << 20, MaxRetainedBytes: 2 << 20, ProcessCapturesPerSecond: 0.5, UploadTimeout: 3 * time.Second}
	var fromFlags profiledump.Config
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	fromFlags.RegisterFlags(flags)
	require.NoError(t, flags.Parse([]string{"-profile-dump.max-object-bytes=1048576", "-profile-dump.max-retained-bytes=2097152", "-profile-dump.process-captures-per-second=0.5", "-profile-dump.upload-timeout=3s"}))
	require.Equal(t, expected, fromFlags)
	fromYAML := profiledump.DefaultConfig()
	decoder := yaml.NewDecoder(strings.NewReader(`max_object_bytes: 1048576
max_retained_bytes: 2097152
process_captures_per_second: 0.5
upload_timeout: 3s
`))
	decoder.KnownFields(true)
	require.NoError(t, decoder.Decode(&fromYAML))
	require.Equal(t, expected, fromYAML)
	require.NoError(t, fromYAML.Recorder.Validate())
	c := validConfig()
	c.ActiveUntil = ptr(now.Add(90 * time.Minute))
	p, err := c.Compile(fromYAML.Recorder.ProcessCapturesPerSecond)
	require.NoError(t, err)
	require.Equal(t, 0.5, p.MaxCapturesPerSecond())
}
