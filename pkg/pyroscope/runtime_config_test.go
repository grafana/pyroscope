package pyroscope

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/runtimeconfig"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/api"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestProfileDumpRuntimeReload(t *testing.T) {
	// Exercise the actual manager, filesystem provider, production loader and
	// tenant adapter together. Deadline boundaries are covered by policy tests.
	now := time.Now().UTC()
	deadline := now.Add(time.Hour)
	path := filepath.Join(t.TempDir(), "runtime.yaml")
	writeConfig := func(contents string) {
		t.Helper()
		tmp := path + ".tmp"
		require.NoError(t, os.WriteFile(tmp, []byte(contents), 0600))
		require.NoError(t, os.Rename(tmp, path))
	}
	initial := fmt.Sprintf(`overrides:
  tenant-a:
    profile_dump:
      active_until: "%s"
      selector: '{service_name=~"checkout.*"}'
      probability: 1
  tenant-b: {}
`, deadline.Format(time.RFC3339Nano))
	writeConfig("overrides: {}\n")
	registry := prometheus.NewRegistry()
	var logs bytes.Buffer
	appLogger := &logger{Logger: log.NewSyncLogger(log.NewLogfmtLogger(&logs))}
	appAPI, err := api.New(api.Config{}, &server.Server{HTTP: mux.NewRouter()}, nil, appLogger)
	require.NoError(t, err)
	app := &Pyroscope{
		Cfg: Config{RuntimeConfig: runtimeconfig.Config{
			LoadPath: []string{path}, ReloadPeriod: 5 * time.Millisecond,
		}},
		logger: appLogger, reg: registry, API: appAPI,
	}
	_, err = app.initRuntimeConfig()
	require.NoError(t, err)
	manager := app.RuntimeConfig
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), manager))
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), manager)) })
	overrides, err := validation.NewOverrides(validation.Limits{}, app.TenantLimits)
	require.NoError(t, err)
	require.Equal(t, profiledump.Policy{}, overrides.ProfileDump("tenant-a"))
	writeConfig(initial)
	require.Eventually(t, func() bool { return overrides.ProfileDump("tenant-a").ActiveAt(now) }, 5*time.Second, time.Millisecond)
	original := overrides.ProfileDump("tenant-a")

	waitPolicy := func(predicate func(profiledump.Policy) bool) {
		t.Helper()
		require.Eventually(t, func() bool { return predicate(overrides.ProfileDump("tenant-a")) }, 5*time.Second, time.Millisecond)
	}
	updated := strings.Replace(initial, "probability: 1", "probability: 0.5", 1)
	writeConfig(updated)
	waitPolicy(func(p profiledump.Policy) bool { return p.Probability() == 0.5 })
	waitReload := func(success float64) {
		t.Helper()
		require.Eventually(t, func() bool {
			families, err := registry.Gather()
			if err != nil {
				return false
			}
			for _, family := range families {
				if family.GetName() == "pyroscope_runtime_config_last_reload_successful" {
					return family.Metric[0].Gauge.GetValue() == success
				}
			}
			return false
		}, 5*time.Second, time.Millisecond)
	}
	// Policy validation and decoding failures both preserve the entire configuration.
	previous := manager.GetConfig()
	previousRate := overrides.IngestionRateBytes("tenant-b")
	for _, invalid := range []string{
		strings.Replace(initial, "probability: 1", "probability: 0", 1),
		strings.Replace(initial, deadline.Format(time.RFC3339Nano), now.Add(25*time.Hour).Format(time.RFC3339Nano), 1),
		strings.Replace(initial, deadline.Format(time.RFC3339Nano), "test", 1),
	} {
		writeConfig(strings.Replace(invalid, "tenant-b: {}", "tenant-b: {ingestion_rate_mb: 17}", 1))
		waitReload(0)
		require.Same(t, previous, manager.GetConfig())
		require.Equal(t, 0.5, overrides.ProfileDump("tenant-a").Probability())
		require.Equal(t, previousRate, overrides.IngestionRateBytes("tenant-b"))
		writeConfig(updated)
		waitReload(1)
	}
	writeConfig("overrides: {}\n")
	require.Eventually(t, func() bool { return manager.GetConfig() != previous }, 5*time.Second, time.Millisecond)
	require.Equal(t, profiledump.Policy{}, overrides.ProfileDump("tenant-a"))
	waitReload(1)
	require.Equal(t, 1.0, original.Probability()) // Old snapshots remain unchanged.
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), manager))
	require.NotContains(t, logs.String(), "level=warn")
	require.Contains(t, logs.String(), "invalid override for tenant tenant-a: profile_dump:")
}

func TestProfileDumpInvalidPolicyFailsStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.yaml")
	require.NoError(t, os.WriteFile(path, []byte("overrides:\n  tenant-a:\n    profile_dump: {}\n"), 0600))
	appLogger := &logger{Logger: log.NewNopLogger()}
	appAPI, err := api.New(api.Config{}, &server.Server{HTTP: mux.NewRouter()}, nil, appLogger)
	require.NoError(t, err)
	app := &Pyroscope{
		Cfg: Config{RuntimeConfig: runtimeconfig.Config{
			LoadPath: []string{path}, ReloadPeriod: time.Second,
		}},
		logger: appLogger, reg: prometheus.NewRegistry(), API: appAPI,
	}
	_, err = app.initRuntimeConfig()
	require.NoError(t, err)
	require.Error(t, services.StartAndAwaitRunning(context.Background(), app.RuntimeConfig))
	require.Equal(t, services.Failed, app.RuntimeConfig.State())
	require.ErrorContains(t, app.RuntimeConfig.FailureCase(), "invalid override for tenant tenant-a: profile_dump: active_until is required")
	require.Nil(t, app.RuntimeConfig.GetConfig())
}

func TestProfileDumpServerConfig(t *testing.T) {
	var defaults Config
	flagext.DefaultValues(&defaults)
	require.Equal(t, profiledump.DefaultConfig(), defaults.ProfileDump)
	cfg := newTestConfig(t, []string{"-profile-dump.max-capture-bytes=1048576", "-profile-dump.max-retained-bytes=2097152", "-profile-dump.retention=24h"})
	require.Equal(t, profiledump.RecorderConfig{MaxCaptureBytes: 1 << 20, MaxRetainedBytes: 2 << 20}, cfg.ProfileDump.Recorder)
	require.Equal(t, 24*time.Hour, cfg.ProfileDump.Cleaner.Retention)
	require.NoError(t, cfg.Validate())
	cfg.ProfileDump.Recorder.MaxCaptureBytes = 0
	require.ErrorContains(t, cfg.Validate(), "profile_dump recorder")
	cfg.ProfileDump.Recorder = defaults.ProfileDump.Recorder
	cfg.ProfileDump.Cleaner.Retention = 0
	require.ErrorContains(t, cfg.Validate(), "profile_dump cleaner")
	cfg.ProfileDump.Cleaner = defaults.ProfileDump.Cleaner
	cfg.LimitsConfig.ProfileDump = &profiledump.TenantConfig{}
	require.ErrorContains(t, cfg.Validate(), "only supported in per-tenant runtime overrides")
}
