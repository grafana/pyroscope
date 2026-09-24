// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/phlareproject/phlare/blob/master/pkg/util/validation/exporter_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The phlare Authors.

package exporter

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv/consul"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/test"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestOverridesExporter_withConfig(t *testing.T) {
	tenantLimits := map[string]*validation.Limits{
		"tenant-a": {
			IngestionRateMB:              10,
			IngestionBurstSizeMB:         11,
			MaxGlobalSeriesPerTenant:     12,
			MaxLocalSeriesPerTenant:      13,
			MaxLabelNameLength:           14,
			MaxLabelValueLength:          15,
			MaxLabelNamesPerSeries:       16,
			MaxQueryLookback:             17,
			MaxQueryLength:               18,
			MaxQueryParallelism:          19,
			QuerySplitDuration:           20,
			MaxSessionsPerSeries:         21,
			DistributorAggregationWindow: 22,
			DistributorAggregationPeriod: 23,
			MaxFlameGraphNodesDefault:    24,
			MaxFlameGraphNodesMax:        25,
		},
	}
	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg1 := Config{RingConfig{}}
	cfg1.Ring.Ring.KVStore.Mock = ringStore
	cfg1.Ring.Ring.InstancePort = 1234
	cfg1.Ring.Ring.HeartbeatPeriod = 15 * time.Second
	cfg1.Ring.Ring.HeartbeatTimeout = 1 * time.Minute

	// Create an empty ring.
	ctx := context.Background()
	require.NoError(t, ringStore.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		return ring.NewDesc(), true, nil
	}))

	// Create an overrides-exporter.
	cfg1.Ring.Ring.InstanceID = "overrides-exporter-1"
	cfg1.Ring.Ring.InstanceAddr = "1.2.3.1"
	exporter, err := NewOverridesExporter(cfg1, &validation.Limits{
		IngestionRateMB:              20,
		IngestionBurstSizeMB:         21,
		MaxGlobalSeriesPerTenant:     22,
		MaxLocalSeriesPerTenant:      23,
		MaxLabelNameLength:           24,
		MaxLabelValueLength:          25,
		MaxLabelNamesPerSeries:       26,
		MaxQueryLookback:             27,
		MaxQueryLength:               28,
		MaxQueryParallelism:          29,
		QuerySplitDuration:           30,
		MaxSessionsPerSeries:         31,
		DistributorAggregationWindow: 32,
		DistributorAggregationPeriod: 33,
		MaxFlameGraphNodesDefault:    34,
		MaxFlameGraphNodesMax:        35,
	}, validation.NewMockTenantLimits(tenantLimits), log.NewNopLogger(), nil)
	require.NoError(t, err)

	l1 := exporter.ring.lifecycler
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), exporter))
	t.Cleanup(func() { assert.NoError(t, services.StopAndAwaitTerminated(context.Background(), exporter)) })

	// Wait until it has received the ring update.
	test.Poll(t, time.Second, true, func() interface{} {
		rs, _ := exporter.ring.client.GetAllHealthy(ringOp)
		return rs.Includes(l1.GetInstanceAddr())
	})

	// Set leader token.
	require.NoError(t, ringStore.CAS(context.Background(), ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters[l1.GetInstanceID()]
		instance.Tokens = []uint32{leaderToken + 1}
		desc.Ingesters[l1.GetInstanceID()] = instance
		return desc, true, nil
	}))

	// Wait for update of token.
	test.Poll(t, time.Second, []uint32{leaderToken + 1}, func() interface{} {
		rs, _ := exporter.ring.client.GetAllHealthy(ringOp)
		return rs.Instances[0].Tokens
	})
	limitsMetrics := `
# HELP pyroscope_limits_overrides Resource limit overrides applied to tenants
# TYPE pyroscope_limits_overrides gauge
pyroscope_limits_overrides{limit_name="profile_debug_dump_active_until_timestamp_seconds",tenant="tenant-a"} 0
pyroscope_limits_overrides{limit_name="profile_debug_dump_active",tenant="tenant-a"} 0
pyroscope_limits_overrides{limit_name="ingestion_rate_mb",tenant="tenant-a"} 10
pyroscope_limits_overrides{limit_name="ingestion_burst_size_mb",tenant="tenant-a"} 11
pyroscope_limits_overrides{limit_name="max_global_series_per_tenant",tenant="tenant-a"} 12
pyroscope_limits_overrides{limit_name="max_series_per_tenant",tenant="tenant-a"} 13
pyroscope_limits_overrides{limit_name="max_label_name_length",tenant="tenant-a"} 14
pyroscope_limits_overrides{limit_name="max_label_value_length",tenant="tenant-a"} 15
pyroscope_limits_overrides{limit_name="max_label_names_per_series",tenant="tenant-a"} 16
pyroscope_limits_overrides{limit_name="max_query_lookback",tenant="tenant-a"} 17
pyroscope_limits_overrides{limit_name="max_query_length",tenant="tenant-a"} 18
pyroscope_limits_overrides{limit_name="max_query_parallelism",tenant="tenant-a"} 19
pyroscope_limits_overrides{limit_name="split_queries_by_interval",tenant="tenant-a"} 20
pyroscope_limits_overrides{limit_name="max_sessions_per_series",tenant="tenant-a"} 21
pyroscope_limits_overrides{limit_name="distributor_aggregation_window",tenant="tenant-a"} 22
pyroscope_limits_overrides{limit_name="distributor_aggregation_period",tenant="tenant-a"} 23
pyroscope_limits_overrides{limit_name="max_flamegraph_nodes_default",tenant="tenant-a"} 24
pyroscope_limits_overrides{limit_name="max_flamegraph_nodes_max",tenant="tenant-a"} 25
`

	// Make sure each override matches the values from the supplied `Limit`
	err = testutil.CollectAndCompare(exporter, bytes.NewBufferString(limitsMetrics), "pyroscope_limits_overrides")
	assert.NoError(t, err)

	limitsMetrics = `
# HELP pyroscope_limits_defaults Resource limit defaults for tenants without overrides
# TYPE pyroscope_limits_defaults gauge
pyroscope_limits_defaults{limit_name="ingestion_rate_mb"} 20
pyroscope_limits_defaults{limit_name="ingestion_burst_size_mb"} 21
pyroscope_limits_defaults{limit_name="max_global_series_per_tenant"} 22
pyroscope_limits_defaults{limit_name="max_series_per_tenant"} 23
pyroscope_limits_defaults{limit_name="max_label_name_length"} 24
pyroscope_limits_defaults{limit_name="max_label_value_length"} 25
pyroscope_limits_defaults{limit_name="max_label_names_per_series"} 26
pyroscope_limits_defaults{limit_name="max_query_lookback"} 27
pyroscope_limits_defaults{limit_name="max_query_length"} 28
pyroscope_limits_defaults{limit_name="max_query_parallelism"} 29
pyroscope_limits_defaults{limit_name="split_queries_by_interval"} 30
pyroscope_limits_defaults{limit_name="max_sessions_per_series"} 31
pyroscope_limits_defaults{limit_name="distributor_aggregation_window"} 32
pyroscope_limits_defaults{limit_name="distributor_aggregation_period"} 33
pyroscope_limits_defaults{limit_name="max_flamegraph_nodes_default"} 34
pyroscope_limits_defaults{limit_name="max_flamegraph_nodes_max"} 35
`
	err = testutil.CollectAndCompare(exporter, bytes.NewBufferString(limitsMetrics), "pyroscope_limits_defaults")
	assert.NoError(t, err)
}

func TestOverridesExporter_withRing(t *testing.T) {
	tenantLimits := map[string]*validation.Limits{
		"tenant-a": {},
	}

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg1 := Config{RingConfig{}}
	cfg1.Ring.Ring.KVStore.Mock = ringStore
	cfg1.Ring.Ring.InstancePort = 1234
	cfg1.Ring.Ring.HeartbeatPeriod = 15 * time.Second
	cfg1.Ring.Ring.HeartbeatTimeout = 1 * time.Minute

	// Create an empty ring.
	ctx := context.Background()
	require.NoError(t, ringStore.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		return ring.NewDesc(), true, nil
	}))

	// Create an overrides-exporter.
	cfg1.Ring.Ring.InstanceID = "overrides-exporter-1"
	cfg1.Ring.Ring.InstanceAddr = "1.2.3.1"
	e1, err := NewOverridesExporter(cfg1, &validation.Limits{}, validation.NewMockTenantLimits(tenantLimits), log.NewNopLogger(), nil)
	l1 := e1.ring.lifecycler
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(ctx, e1))
	t.Cleanup(func() { assert.NoError(t, services.StopAndAwaitTerminated(ctx, e1)) })

	// Wait until it has received the ring update.
	test.Poll(t, time.Second, true, func() interface{} {
		rs, _ := e1.ring.client.GetAllHealthy(ringOp)
		return rs.Includes(l1.GetInstanceAddr())
	})

	// Set leader token.
	require.NoError(t, ringStore.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters[l1.GetInstanceID()]
		instance.Tokens = []uint32{leaderToken + 1}
		desc.Ingesters[l1.GetInstanceID()] = instance
		return desc, true, nil
	}))

	// Wait for update of token.
	test.Poll(t, time.Second, []uint32{leaderToken + 1}, func() interface{} {
		rs, _ := e1.ring.client.GetAllHealthy(ringOp)
		return rs.Instances[0].Tokens
	})

	// This instance is now the only ring member and should export metrics.
	require.True(t, hasOverrideMetrics(e1))

	// Register a second instance.
	cfg2 := cfg1
	cfg2.Ring.Ring.InstanceID = "overrides-exporter-2"
	cfg2.Ring.Ring.InstanceAddr = "1.2.3.2"
	e2, err := NewOverridesExporter(cfg2, &validation.Limits{}, validation.NewMockTenantLimits(tenantLimits), log.NewNopLogger(), nil)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(ctx, e2))
	t.Cleanup(func() { assert.NoError(t, services.StopAndAwaitTerminated(ctx, e2)) })

	// Wait until it has registered itself to the ring and both overrides-exporter instances got the updated ring.
	test.Poll(t, time.Second, true, func() interface{} {
		rs1, _ := e1.ring.client.GetAllHealthy(ringOp)
		rs2, _ := e2.ring.client.GetAllHealthy(ringOp)
		return rs1.Includes(e2.ring.lifecycler.GetInstanceAddr()) && rs2.Includes(e1.ring.lifecycler.GetInstanceAddr())
	})

	// Only the leader instance (owner of the special token) should export metrics.
	require.True(t, hasOverrideMetrics(e1))
	require.False(t, hasOverrideMetrics(e2))
}

func hasOverrideMetrics(e1 prometheus.Collector) bool {
	return testutil.CollectAndCount(e1, "pyroscope_limits_overrides") > 0
}

func TestOverridesExporter_ProfileDebugDump(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(time.Minute + 500*time.Millisecond)
	cfg, err := validation.LoadRuntimeConfigWithProfileDump(strings.NewReader(`overrides:
  active:
    profile_debug_dump:
      active_until: "2026-09-24T12:01:00.5Z"
      probability: 1
  also-active:
    profile_debug_dump:
      active_until: "2026-09-24T12:01:00.5Z"
      probability: 1
  past:
    profile_debug_dump:
      active_until: "0001-01-01T00:00:00Z"
      probability: 1
  absent: {}
  null-policy:
    profile_debug_dump: null
`), profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	exporterConfig := Config{}
	exporterConfig.Ring.Ring.KVStore.Mock = ringStore
	exporterConfig.Ring.Ring.InstanceAddr = "127.0.0.1"
	e, err := NewOverridesExporter(exporterConfig, &validation.Limits{}, validation.NewMockTenantLimits(cfg.TenantLimits), log.NewNopLogger(), nil)
	require.NoError(t, err)
	e.ring = nil
	calls := 0
	e.now = func() time.Time {
		calls++
		return now
	}
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(e)
	gather := func() map[string]map[string]float64 {
		t.Helper()
		before := calls
		families, err := reg.Gather()
		require.NoError(t, err)
		require.Equal(t, before+1, calls, "sample time once per collection")
		values := map[string]map[string]float64{}
		for _, family := range families {
			for _, metric := range family.Metric {
				var tenant, name string
				for _, label := range metric.Label {
					switch label.GetName() {
					case "tenant":
						tenant = label.GetValue()
					case "limit_name":
						name = label.GetValue()
					}
				}
				if !strings.HasPrefix(name, "profile_debug_dump_") {
					continue
				}
				require.Equal(t, "pyroscope_limits_overrides", family.GetName(), "capture policy is runtime-only")
				if values[tenant] == nil {
					values[tenant] = map[string]float64{}
				}
				values[tenant][name] = metric.GetGauge().GetValue()
			}
		}
		return values
	}
	disabled := map[string]float64{
		"profile_debug_dump_active_until_timestamp_seconds": 0,
		"profile_debug_dump_active":                         0,
	}
	active := map[string]float64{
		"profile_debug_dump_active_until_timestamp_seconds": float64(deadline.Unix()) + 0.5,
		"profile_debug_dump_active":                         1,
	}
	require.Equal(t, map[string]map[string]float64{
		"active": active, "also-active": active, "absent": disabled, "null-policy": disabled,
		"past": {
			"profile_debug_dump_active_until_timestamp_seconds": float64(time.Time{}.Unix()),
			"profile_debug_dump_active":                         0,
		},
	}, gather())

	// Published policy values do not follow mutations of the input configuration.
	*cfg.TenantLimits["active"].ProfileDebugDump.ActiveUntil = now.Add(-time.Hour)
	require.Equal(t, active, gather()["active"])

	now = deadline
	expired := map[string]float64{
		"profile_debug_dump_active_until_timestamp_seconds": active["profile_debug_dump_active_until_timestamp_seconds"],
		"profile_debug_dump_active":                         0,
	}
	require.Equal(t, expired, gather()["active"])
	require.Equal(t, expired, gather()["also-active"])

	replacement, err := validation.LoadRuntimeConfigWithProfileDump(strings.NewReader("overrides:\n  active: {}\n"), profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	cfg.TenantLimits["active"] = replacement.TenantLimits["active"]
	require.Equal(t, disabled, gather()["active"])
	delete(cfg.TenantLimits, "active")
	require.NotContains(t, gather(), "active")

	e.tenantLimits = nil
	callsBefore := calls
	families, err := reg.Gather()
	require.NoError(t, err)
	require.Equal(t, callsBefore, calls)
	require.Len(t, families, 1)
	require.Equal(t, "pyroscope_limits_defaults", families[0].GetName())
}
