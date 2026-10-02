package pyroscope

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/runtimeconfig"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestProfileDebugDumpRuntimeReload(t *testing.T) {
	// Exercise the actual manager, filesystem provider, production loader and
	// tenant adapter together. Only the clock is substituted.
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	deadline := now.Add(time.Minute)
	path := filepath.Join(t.TempDir(), "runtime.yaml")
	writeConfig := func(contents string) {
		t.Helper()
		tmp := path + ".tmp"
		require.NoError(t, os.WriteFile(tmp, []byte(contents), 0600))
		require.NoError(t, os.Rename(tmp, path))
	}
	const initial = `overrides:
  tenant-a:
    profile_debug_dump:
      active_until: "2026-09-16T12:01:00Z"
      selector: '{service_name=~"checkout.*"}'
      probability: 1
  tenant-b: {}
`
	writeConfig(initial)
	registry := prometheus.NewRegistry()
	startManager := func() *runtimeconfig.Manager {
		t.Helper()
		cfg := runtimeconfig.Config{
			LoadPath: []string{path}, ReloadPeriod: 5 * time.Millisecond,
			Loader: func(r io.Reader) (interface{}, error) {
				return validation.LoadRuntimeConfigWithProfileDump(r, profiledump.DefaultConfig(), time.Unix(0, clock.Load()))
			},
		}
		manager, err := runtimeconfig.New(cfg, "profile-dump-test", registry, log.NewNopLogger())
		require.NoError(t, err)
		require.NoError(t, services.StartAndAwaitRunning(context.Background(), manager))
		t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), manager)) })
		return manager
	}
	manager := startManager()
	overrides, err := validation.NewOverrides(validation.Limits{}, newTenantLimits(manager))
	require.NoError(t, err)
	original := overrides.ProfileDebugDump("tenant-a")
	require.True(t, original.ActiveAt(now))
	require.Equal(t, deadline, original.ActiveUntil())
	require.False(t, overrides.ProfileDebugDump("tenant-b").ActiveAt(now))
	require.False(t, overrides.ProfileDebugDump("unknown").ActiveAt(now))

	// Readers run throughout successful reloads, rejected reloads and removal.
	ctx, cancel := context.WithCancel(context.Background())
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for ctx.Err() == nil {
				p := overrides.ProfileDebugDump("tenant-a")
				if p.Fingerprint() != "" {
					if !p.ActiveUntil().Equal(deadline) || (p.Probability() != 1 && p.Probability() != 0.5) {
						t.Error("reader observed a partial policy")
						return
					}
					if !p.Matches(labels.FromStrings("service_name", "checkout-api")) {
						t.Error("compiled selector changed during read")
						return
					}
				}
				_ = p.ActiveAt(time.Unix(0, clock.Load()))
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		readers.Wait()
	})
	waitPolicy := func(predicate func(profiledump.Policy) bool) {
		t.Helper()
		require.Eventually(t, func() bool { return predicate(overrides.ProfileDebugDump("tenant-a")) }, 5*time.Second, time.Millisecond)
	}
	clock.Store(now.Add(30 * time.Second).UnixNano())
	// Force a reload with identical effective policy, rather than a hash-cache hit.
	previous := manager.GetConfig()
	writeConfig(strings.Replace(initial, "probability: 1", "probability: 1\n      max_captures_per_second: 1", 1))
	require.Eventually(t, func() bool { return manager.GetConfig() != previous }, 5*time.Second, time.Millisecond)
	require.Equal(t, original.Fingerprint(), overrides.ProfileDebugDump("tenant-a").Fingerprint())
	require.Equal(t, deadline, overrides.ProfileDebugDump("tenant-a").ActiveUntil())

	writeConfig(strings.Replace(initial, "probability: 1", "probability: 0.5", 1))
	waitPolicy(func(p profiledump.Policy) bool { return p.Probability() == 0.5 })
	previous = manager.GetConfig()
	retained := overrides.ProfileDebugDump("tenant-a")
	require.NotEqual(t, original.Fingerprint(), retained.Fingerprint())
	// Reject the entire reload even if another tenant was validated first.
	writeConfig(strings.Replace(initial, "tenant-b: {}", "tenant-b:\n    profile_debug_dump: {probability: 1}", 1))
	require.Eventually(t, func() bool {
		families, err := registry.Gather()
		if err != nil {
			return false
		}
		for _, family := range families {
			if family.GetName() == "runtime_config_last_reload_successful" {
				return family.Metric[0].Gauge.GetValue() == 0
			}
		}
		return false
	}, 5*time.Second, time.Millisecond)
	require.Same(t, previous, manager.GetConfig())
	require.Equal(t, retained.Fingerprint(), overrides.ProfileDebugDump("tenant-a").Fingerprint())
	for _, at := range []time.Time{deadline.Add(-time.Nanosecond), deadline, deadline.Add(time.Nanosecond)} {
		clock.Store(at.UnixNano())
		p := overrides.ProfileDebugDump("tenant-a")
		require.Equal(t, at.Before(deadline), p.ActiveAt(at))
		require.Equal(t, deadline, p.ActiveUntil())
	}

	// Removal of a block and removal of a tenant both disable future admissions.
	for _, removed := range []string{"overrides:\n  tenant-a: {}\n", "overrides:\n  tenant-a:\n    profile_debug_dump: null\n", "overrides: {}\n"} {
		writeConfig(initial)
		waitPolicy(func(p profiledump.Policy) bool { return p.Fingerprint() == original.Fingerprint() })
		writeConfig(removed)
		waitPolicy(func(p profiledump.Policy) bool { return p.Fingerprint() == "" })
	}
	require.Equal(t, 1.0, original.Probability()) // Old snapshots remain unchanged.
	cancel()
	readers.Wait()

	// Starting a new process from the same file cannot renew an expired deadline.
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), manager))
	writeConfig(initial)
	registry = prometheus.NewRegistry()
	restarted := startManager()
	restartedOverrides, err := validation.NewOverrides(validation.Limits{}, newTenantLimits(restarted))
	require.NoError(t, err)
	p := restartedOverrides.ProfileDebugDump("tenant-a")
	require.Equal(t, original.Fingerprint(), p.Fingerprint())
	require.Equal(t, deadline, p.ActiveUntil())
	require.False(t, p.ActiveAt(time.Unix(0, clock.Load())))
}

func TestProfileDumpServerConfig(t *testing.T) {
	var cfg Config
	flagext.DefaultValues(&cfg)
	require.Equal(t, profiledump.DefaultConfig(), cfg.ProfileDump)
	require.NoError(t, yaml.Unmarshal([]byte(`profile_dump:
  max_activation_window: 2h
  default_captures_per_second: 2
  max_captures_per_second: 20
`), &cfg))
	require.Equal(t, 2*time.Hour, cfg.ProfileDump.MaxActivationWindow)
	require.Equal(t, 2.0, cfg.ProfileDump.DefaultCapturesPerSecond)
	require.Equal(t, 20.0, cfg.ProfileDump.MaxCapturesPerSecond)
	cfg.ProfileDump.MaxActivationWindow = 0
	require.ErrorContains(t, cfg.Validate(), "max_activation_window")
	cfg.ProfileDump = profiledump.DefaultConfig()
	cfg.LimitsConfig.ProfileDebugDump = &profiledump.TenantConfig{}
	require.ErrorContains(t, cfg.Validate(), "only supported in per-tenant runtime overrides")
}
