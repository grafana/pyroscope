package validation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

func TestProfileDebugDumpRuntimeConfig(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	const deadline = `active_until: "2026-09-16T12:01:00Z"`
	tests := []struct{ name, block, wantErr string }{
		{"absent", "", ""},
		{"null", "profile_debug_dump: null", ""},
		{"empty", "profile_debug_dump: {}", "active_until is required"},
		{"missing deadline", "profile_debug_dump: {probability: 1}", "active_until is required"},
		{"null deadline", "profile_debug_dump: {active_until: null, probability: 1}", "active_until is required"},
		{"malformed deadline", `profile_debug_dump: {active_until: "bad", probability: 1}`, "cannot parse"},
		{"missing probability", "profile_debug_dump: {" + deadline + "}", "probability is required"},
		{"null probability", "profile_debug_dump: {" + deadline + ", probability: null}", "probability is required"},
		{"zero probability", "profile_debug_dump: {" + deadline + ", probability: 0}", "probability must"},
		{"nan probability", "profile_debug_dump: {" + deadline + ", probability: .nan}", "probability must"},
		{"inf probability", "profile_debug_dump: {" + deadline + ", probability: .inf}", "probability must"},
		{"negative inf probability", "profile_debug_dump: {" + deadline + ", probability: -.inf}", "probability must"},
		{"invalid rate", "profile_debug_dump: {" + deadline + ", probability: 1, max_captures_per_second: 0}", "max_captures_per_second"},
		{"inf rate", "profile_debug_dump: {" + deadline + ", probability: 1, max_captures_per_second: .inf}", "max_captures_per_second"},
		{"over ceiling", "profile_debug_dump: {" + deadline + ", probability: 1, max_captures_per_second: 11}", "max_captures_per_second"},
		{"unknown field", "profile_debug_dump: {" + deadline + ", probability: 1, typo: true}", "field typo"},
		{"invalid selector", "profile_debug_dump: {" + deadline + ", probability: 1, selector: 'bad{'}", "invalid selector"},
		{"valid", "profile_debug_dump: {" + deadline + ", probability: 1}", ""},
		{"past", `profile_debug_dump: {active_until: "2026-09-15T12:00:00Z", probability: 1}`, ""},
		{"over window", `profile_debug_dump: {active_until: "2026-09-16T13:00:01Z", probability: 1}`, "maximum activation window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadRuntimeConfigWithProfileDump(strings.NewReader("overrides:\n  tenant-a:\n    "+tt.block+"\n  tenant-b: {}\n"), profiledump.DefaultConfig(), now)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			overrides, err := NewOverrides(Limits{}, NewMockTenantLimits(cfg.TenantLimits))
			require.NoError(t, err)
			p := overrides.ProfileDebugDump("tenant-a")
			require.Equal(t, tt.name == "valid", p.ActiveAt(now))
			require.False(t, overrides.ProfileDebugDump("tenant-b").ActiveAt(now))
			require.False(t, overrides.ProfileDebugDump("unknown").ActiveAt(now))
			if tt.name != "valid" {
				return
			}
			require.Equal(t, "{}", p.Selector())
			require.Equal(t, 1.0, p.MaxCapturesPerSecond())
			// Native Limits YAML/JSON decoding preserves omission and requires explicit validation.
			for _, codec := range []struct {
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
			}{{yaml.Marshal, yaml.Unmarshal}, {json.Marshal, json.Unmarshal}} {
				b, err := codec.marshal(cfg.TenantLimits["tenant-a"])
				require.NoError(t, err)
				var decoded Limits
				require.NoError(t, codec.unmarshal(b, &decoded))
				require.NoError(t, decoded.Validate(profiledump.DefaultConfig(), now))
				require.Equal(t, p.Fingerprint(), decoded.profileDebugDumpPolicy.Fingerprint())
			}
			*cfg.TenantLimits["tenant-a"].ProfileDebugDump.Probability = 0.1
			require.Equal(t, 1.0, overrides.ProfileDebugDump("tenant-a").Probability())
			require.True(t, p.Matches(labels.EmptyLabels()))
		})
	}
}

func TestProfileDebugDumpExplicitValidationBounds(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	deadline, probability := now.Add(2*time.Hour), 1.0
	bounds := profiledump.DefaultConfig()
	bounds.MaxActivationWindow = 3 * time.Hour
	bounds.DefaultCapturesPerSecond = 0.5
	limits := Limits{ProfileDebugDump: &profiledump.TenantConfig{ActiveUntil: &deadline, Probability: &probability}}
	require.ErrorContains(t, limits.Validate(profiledump.DefaultConfig(), now), "maximum activation window")
	require.NoError(t, limits.Validate(bounds, now))
	require.Equal(t, 0.5, limits.profileDebugDumpPolicy.MaxCapturesPerSecond())
	input := "overrides:\n  tenant-a:\n    profile_debug_dump: {active_until: \"2026-09-16T14:00:00Z\", probability: 1}\n"
	rejected, err := LoadRuntimeConfigWithProfileDump(strings.NewReader(input), profiledump.DefaultConfig(), now)
	require.ErrorContains(t, err, "maximum activation window")
	require.Nil(t, rejected)
	config, err := LoadRuntimeConfigWithProfileDump(strings.NewReader(input), bounds, now)
	require.NoError(t, err)
	require.Equal(t, limits.profileDebugDumpPolicy.Fingerprint(), config.TenantLimits["tenant-a"].profileDebugDumpPolicy.Fingerprint())
}

func TestProfileDebugDumpNotInherited(t *testing.T) {
	previous := defaultLimits
	t.Cleanup(func() { defaultLimits = previous })
	var defaults Limits
	require.NoError(t, yaml.Unmarshal([]byte(`profile_debug_dump: {active_until: "2026-09-16T12:01:00Z", probability: 1}`), &defaults))
	SetDefaultLimitsForYAMLUnmarshalling(defaults)
	for _, input := range []string{"{}", "profile_debug_dump: null"} {
		var l Limits
		require.NoError(t, yaml.Unmarshal([]byte(input), &l))
		require.Nil(t, l.ProfileDebugDump)
	}
}

func TestProfileDebugDumpTenantIsolation(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	const input = `overrides:
  tenant-a:
    profile_debug_dump:
      active_until: "2026-09-16T12:01:00Z"
      probability: 0.25
      selector: '{service_name="checkout"}'
  tenant-b:
    profile_debug_dump:
      active_until: "2026-09-16T12:02:00Z"
      probability: 0.5
      selector: '{service_name="payments"}'
      max_captures_per_second: 4
`
	cfg, err := LoadRuntimeConfigWithProfileDump(strings.NewReader(input), profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	o, err := NewOverrides(Limits{}, NewMockTenantLimits(cfg.TenantLimits))
	require.NoError(t, err)
	a, b := o.ProfileDebugDump("tenant-a"), o.ProfileDebugDump("tenant-b")
	require.Equal(t, 0.25, a.Probability())
	require.Equal(t, 0.5, b.Probability())
	require.Equal(t, 1.0, a.MaxCapturesPerSecond())
	require.Equal(t, 4.0, b.MaxCapturesPerSecond())
	require.NotEqual(t, a.Fingerprint(), b.Fingerprint())
	checkout := labels.FromStrings("service_name", "checkout")
	require.True(t, a.Matches(checkout))
	require.False(t, b.Matches(checkout))
	// The accessor never reparses or refers to the mutable configuration fields.
	*cfg.TenantLimits["tenant-a"].ProfileDebugDump.Selector = "invalid{"
	a = profiledump.Policy{}
	require.False(t, a.ActiveAt(now))
	require.True(t, o.ProfileDebugDump("tenant-a").Matches(checkout))
	require.False(t, o.ProfileDebugDump("tenant-b").Matches(checkout))

	// The global ceiling cannot be raised by a runtime tenant override.
	_, err = LoadRuntimeConfigWithProfileDump(strings.NewReader("overrides:\n  tenant-a:\n    profile_dump: {max_captures_per_second: 100}\n"), profiledump.DefaultConfig(), now)
	require.ErrorContains(t, err, "field profile_dump")
	bounds := profiledump.DefaultConfig()
	bounds.MaxCapturesPerSecond = 2
	_, err = LoadRuntimeConfigWithProfileDump(strings.NewReader(input), bounds, now)
	require.ErrorContains(t, err, "invalid override for tenant tenant-b")
	bounds.MaxActivationWindow = 0
	_, err = LoadRuntimeConfigWithProfileDump(strings.NewReader("overrides: {}"), bounds, now)
	require.ErrorContains(t, err, "max_activation_window")
}
