package validation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

func TestProfileDumpRuntimeConfig(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	const deadline = `active_until: "2026-09-16T12:01:00Z"`
	tests := []struct{ name, block, wantErr string }{
		{"absent", "{}", ""},
		{"null", "profile_dump: null", ""},
		{"malformed deadline", `profile_dump: {active_until: "test", probability: 1}`, "cannot parse"},
		{"unknown field", "profile_dump: {" + deadline + ", probability: 1, typo: true}", "field typo"},
		{"unconvertible probability", "profile_dump: {" + deadline + ", probability: []}", "cannot unmarshal"},
		{"valid", "profile_dump: {" + deadline + ", probability: 1}", ""},
		{"invalid probability", "profile_dump: {" + deadline + ", probability: 0}", ""},
		{"invalid unrelated field", `ingestion_relabeling_default_rules_position: invalid
    profile_dump: {active_until: "2036-09-16T12:01:00Z", probability: 1}`, "invalid"},
		{"malformed YAML", "profile_dump: [", "yaml:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadRuntimeConfig(strings.NewReader("overrides:\n  tenant-a:\n    "+tt.block+"\n  tenant-b: {}\n"), now)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, cfg)
				return
			}
			require.NoError(t, err)
			overrides, err := NewOverrides(Limits{}, NewMockTenantLimits(cfg.TenantLimits))
			require.NoError(t, err)
			p := overrides.ProfileDump("tenant-a")
			require.Equal(t, tt.name == "valid", p.ActiveAt(now))
		})
	}
	overrides, err := NewOverrides(Limits{}, NewMockTenantLimits(map[string]*Limits{"tenant-b": {}}))
	require.NoError(t, err)
	require.False(t, overrides.ProfileDump("tenant-b").ActiveAt(now))
	require.False(t, overrides.ProfileDump("unknown").ActiveAt(now))
}

func TestProfileDumpTenantIsolation(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	const input = `overrides:
  tenant-a:
    profile_dump:
      active_until: "2026-09-16T12:01:00Z"
      probability: 0.25
      selector: '{service_name="checkout"}'
  tenant-b:
    profile_dump:
      active_until: "2026-09-16T12:02:00Z"
      probability: 0.5
      selector: '{service_name="payments"}'
`
	cfg, err := loadRuntimeConfig(strings.NewReader(input), now)
	require.NoError(t, err)
	o, err := NewOverrides(Limits{}, NewMockTenantLimits(cfg.TenantLimits))
	require.NoError(t, err)
	a, b := o.ProfileDump("tenant-a"), o.ProfileDump("tenant-b")
	require.Equal(t, 0.25, a.Probability())
	require.Equal(t, 0.5, b.Probability())
	require.NotEqual(t, a.ActiveUntil(), b.ActiveUntil())
	checkout := model.LabelsFromStrings("service_name", "checkout")
	require.True(t, a.Matches(checkout))
	require.False(t, b.Matches(checkout))
	// The accessor never reparses or refers to the mutable configuration fields.
	*cfg.TenantLimits["tenant-a"].ProfileDump.Selector = "invalid{"
	require.True(t, o.ProfileDump("tenant-a").Matches(checkout))
	require.False(t, o.ProfileDump("tenant-b").Matches(checkout))
	require.NoError(t, cfg.TenantLimits["tenant-a"].validate(now))
	require.Equal(t, profiledump.Policy{}, o.ProfileDump("tenant-a"))
	require.Equal(t, b, o.ProfileDump("tenant-b"))
}

func TestProfileDumpDeadlineRevalidation(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(25 * time.Hour)
	input := fmt.Sprintf("overrides:\n  tenant-a:\n    profile_dump: {active_until: %q, probability: 1}\n", deadline.Format(time.RFC3339))
	cfg, err := loadRuntimeConfig(strings.NewReader(input), now)
	require.NoError(t, err)
	policy := cfg.TenantLimits["tenant-a"].ProfileDumpPolicy()
	require.Equal(t, profiledump.Policy{}, policy)
	require.False(t, policy.ActiveAt(now.Add(time.Hour)), "time passing does not activate a disabled snapshot")
	for _, at := range []time.Time{now.Add(time.Hour), deadline} {
		reloaded, err := loadRuntimeConfig(strings.NewReader(input), at)
		require.NoError(t, err)
		p := reloaded.TenantLimits["tenant-a"].profileDumpPolicy
		require.Equal(t, deadline, p.ActiveUntil())
		require.Equal(t, at.Before(deadline), p.ActiveAt(at))
	}
}
