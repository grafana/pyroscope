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
	// tenant adapter together. Policy expiry is checked at explicit times.
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	deadline := now.AddDate(10, 0, 0).Add(time.Minute)
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
      active_until: "2036-09-16T12:01:00Z"
      selector: '{service_name=~"checkout.*"}'
      probability: 1
  tenant-b: {}
`
	writeConfig(initial)
	registry := prometheus.NewRegistry()
	processRate := 0.5
	startManager := func() *runtimeconfig.Manager {
		t.Helper()
		cfg := runtimeconfig.Config{
			LoadPath: []string{path}, ReloadPeriod: 5 * time.Millisecond,
			Loader: func(r io.Reader) (interface{}, error) {
				return validation.LoadRuntimeConfigWithProfileDump(r, processRate)
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
	require.Equal(t, 0.5, original.MaxCapturesPerSecond())
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
				if p.Probability() > 0 {
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
	writeConfig(strings.Replace(initial, "probability: 1", "probability: 1\n      max_captures_per_second: 0.5", 1))
	require.Eventually(t, func() bool { return manager.GetConfig() != previous }, 5*time.Second, time.Millisecond)
	require.True(t, overrides.ProfileDebugDump("tenant-a").Matches(labels.FromStrings("service_name", "checkout-api")))
	require.False(t, overrides.ProfileDebugDump("tenant-a").Matches(labels.FromStrings("service_name", "payments")))
	require.Equal(t, original.Probability(), overrides.ProfileDebugDump("tenant-a").Probability())
	require.Equal(t, original.MaxCapturesPerSecond(), overrides.ProfileDebugDump("tenant-a").MaxCapturesPerSecond())
	require.Equal(t, deadline, overrides.ProfileDebugDump("tenant-a").ActiveUntil())

	updated := strings.Replace(initial, "probability: 1", "probability: 0.5\n      max_captures_per_second: 5", 1)
	writeConfig(updated)
	waitPolicy(func(p profiledump.Policy) bool { return p.Probability() == 0.5 && p.MaxCapturesPerSecond() == 5 })
	require.Equal(t, deadline, overrides.ProfileDebugDump("tenant-a").ActiveUntil())
	previous = manager.GetConfig()
	retained := overrides.ProfileDebugDump("tenant-a")
	require.Equal(t, 0.5, retained.Probability())
	waitReload := func(success float64) {
		t.Helper()
		require.Eventually(t, func() bool {
			families, err := registry.Gather()
			if err != nil {
				return false
			}
			for _, family := range families {
				if family.GetName() == "runtime_config_last_reload_successful" {
					return family.Metric[0].Gauge.GetValue() == success
				}
			}
			return false
		}, 5*time.Second, time.Millisecond)
	}
	// Reject the entire reload even if another tenant was validated first.
	for _, invalid := range []string{
		strings.Replace(strings.Replace(initial, `{service_name=~"checkout.*"}`, `{service_name="payments"}`, 1), "tenant-b: {}", "tenant-b:\n    profile_debug_dump: {probability: 1}", 1),
		strings.Replace(initial, "probability: 1", "probability: 1\n      max_captures_per_second: .nan", 1),
	} {
		// Restore success so the failure metric proves this candidate was read.
		writeConfig(updated)
		waitReload(1)
		previous = manager.GetConfig()
		writeConfig(invalid)
		waitReload(0)
		require.Same(t, previous, manager.GetConfig())
		require.Equal(t, retained.ActiveUntil(), overrides.ProfileDebugDump("tenant-a").ActiveUntil())
		require.Equal(t, retained.Probability(), overrides.ProfileDebugDump("tenant-a").Probability())
		p := overrides.ProfileDebugDump("tenant-a")
		require.Equal(t, retained.MaxCapturesPerSecond(), p.MaxCapturesPerSecond())
		require.True(t, p.ActiveAt(now))
		require.True(t, p.Matches(labels.FromStrings("service_name", "checkout-api")))
		require.False(t, p.Matches(labels.FromStrings("service_name", "payments")))
	}
	for _, at := range []time.Time{deadline.Add(-time.Nanosecond), deadline, deadline.Add(time.Nanosecond)} {
		clock.Store(at.UnixNano())
		p := overrides.ProfileDebugDump("tenant-a")
		require.Equal(t, at.Before(deadline), p.ActiveAt(at))
		require.Equal(t, deadline, p.ActiveUntil())
	}

	// Removal of a block and removal of a tenant both disable future admissions.
	for _, removed := range []string{"overrides:\n  tenant-a: {}\n", "overrides:\n  tenant-a:\n    profile_debug_dump: null\n", "overrides: {}\n"} {
		writeConfig(initial)
		waitPolicy(func(p profiledump.Policy) bool {
			return p.Probability() == original.Probability() &&
				p.Matches(labels.FromStrings("service_name", "checkout-api")) &&
				!p.Matches(labels.FromStrings("service_name", "payments")) &&
				p.ActiveUntil().Equal(original.ActiveUntil()) && p.MaxCapturesPerSecond() == original.MaxCapturesPerSecond()
		})
		writeConfig(removed)
		waitPolicy(func(p profiledump.Policy) bool { return p.Probability() == 0 })
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
	require.True(t, p.Matches(labels.FromStrings("service_name", "checkout-api")))
	require.False(t, p.Matches(labels.FromStrings("service_name", "payments")))
	require.Equal(t, original.Probability(), p.Probability())
	require.Equal(t, original.MaxCapturesPerSecond(), p.MaxCapturesPerSecond())
	require.Equal(t, deadline, p.ActiveUntil())
	require.False(t, p.ActiveAt(time.Unix(0, clock.Load())))
}

func TestProfileDumpServerConfig(t *testing.T) {
	var cfg Config
	flagext.DefaultValues(&cfg)
	require.Equal(t, profiledump.DefaultConfig(), cfg.ProfileDump)
	require.NoError(t, yaml.Unmarshal([]byte(`profile_dump:
  process_captures_per_second: 0.5
`), &cfg))
	require.Equal(t, 0.5, cfg.ProfileDump.Recorder.ProcessCapturesPerSecond)
	cfg.ProfileDump.Recorder.ProcessCapturesPerSecond = 0
	require.ErrorContains(t, cfg.Validate(), "recorder rate and durations must be finite and positive")
	cfg.ProfileDump = profiledump.DefaultConfig()
	cfg.LimitsConfig.ProfileDebugDump = &profiledump.TenantConfig{}
	require.ErrorContains(t, cfg.Validate(), "only supported in per-tenant runtime overrides")
}

func TestProfileDebugDumpInvalidInitialLoad(t *testing.T) {
	for _, block := range []string{
		`{probability: 1}`,
		`{active_until: "2036-09-16T12:01:00Z", probability: 0}`,
		`{active_until: "2036-09-16T12:01:00Z", probability: .inf}`,
		`{active_until: "2036-09-16T12:01:00Z", probability: 1, selector: "bad{"}`,
		`{active_until: "2036-09-16T12:01:00Z", probability: 1, max_captures_per_second: .nan}`,
	} {
		t.Run(block, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtime.yaml")
			require.NoError(t, os.WriteFile(path, []byte("overrides:\n  tenant-a:\n    profile_debug_dump: "+block+"\n"), 0600))
			processRate := 0.5
			manager, err := runtimeconfig.New(runtimeconfig.Config{
				LoadPath: []string{path}, ReloadPeriod: time.Hour,
				Loader: func(r io.Reader) (interface{}, error) {
					return validation.LoadRuntimeConfigWithProfileDump(r, processRate)
				},
			}, "profile-dump-initial-test", prometheus.NewRegistry(), log.NewNopLogger())
			require.NoError(t, err)
			require.Error(t, services.StartAndAwaitRunning(context.Background(), manager))
			require.Equal(t, services.Failed, manager.State())
			require.Nil(t, manager.GetConfig())
		})
	}
}
