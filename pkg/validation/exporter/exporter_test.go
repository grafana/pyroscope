// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/phlareproject/phlare/blob/master/pkg/util/validation/exporter_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The phlare Authors.

package exporter

import (
	"bytes"
	"context"
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

	"github.com/grafana/phlare/pkg/validation"
)

func TestOverridesExporter_withConfig(t *testing.T) {
	tenantLimits := map[string]*validation.Limits{
		"tenant-a": {
			IngestionRateMB:          10,
			IngestionBurstSizeMB:     11,
			MaxGlobalSeriesPerTenant: 12,
			MaxLocalSeriesPerTenant:  13,
			MaxLabelNameLength:       14,
			MaxLabelValueLength:      15,
			MaxLabelNamesPerSeries:   16,
			MaxQueryLookback:         17,
			MaxQueryLength:           18,
			MaxQueryParallelism:      19,
		},
	}
	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg1 := Config{RingConfig{}}
	cfg1.Ring.KVStore.Mock = ringStore
	cfg1.Ring.InstancePort = 1234
	cfg1.Ring.HeartbeatPeriod = 15 * time.Second
	cfg1.Ring.HeartbeatTimeout = 1 * time.Minute

	// Create an empty ring.
	ctx := context.Background()
	require.NoError(t, ringStore.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		return ring.NewDesc(), true, nil
	}))

	// Create an overrides-exporter.
	cfg1.Ring.InstanceID = "overrides-exporter-1"
	cfg1.Ring.InstanceAddr = "1.2.3.1"
	exporter, err := NewOverridesExporter(cfg1, &validation.Limits{
		IngestionRateMB:          20,
		IngestionBurstSizeMB:     21,
		MaxGlobalSeriesPerTenant: 22,
		MaxLocalSeriesPerTenant:  23,
		MaxLabelNameLength:       24,
		MaxLabelValueLength:      25,
		MaxLabelNamesPerSeries:   26,
		MaxQueryLookback:         27,
		MaxQueryLength:           28,
		MaxQueryParallelism:      29,
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
# HELP phlare_limits_overrides Resource limit overrides applied to tenants
# TYPE phlare_limits_overrides gauge
phlare_limits_overrides{limit_name="ingestion_rate_mb",tenant="tenant-a"} 10
phlare_limits_overrides{limit_name="ingestion_burst_size_mb",tenant="tenant-a"} 11
phlare_limits_overrides{limit_name="max_global_series_per_tenant",tenant="tenant-a"} 12
phlare_limits_overrides{limit_name="max_series_per_tenant",tenant="tenant-a"} 13
phlare_limits_overrides{limit_name="max_label_name_length",tenant="tenant-a"} 14
phlare_limits_overrides{limit_name="max_label_value_length",tenant="tenant-a"} 15
phlare_limits_overrides{limit_name="max_label_names_per_series",tenant="tenant-a"} 16
phlare_limits_overrides{limit_name="max_query_lookback",tenant="tenant-a"} 17
phlare_limits_overrides{limit_name="max_query_length",tenant="tenant-a"} 18
phlare_limits_overrides{limit_name="max_query_parallelism",tenant="tenant-a"} 19
`

	// Make sure each override matches the values from the supplied `Limit`
	err = testutil.CollectAndCompare(exporter, bytes.NewBufferString(limitsMetrics), "phlare_limits_overrides")
	assert.NoError(t, err)

	limitsMetrics = `
# HELP phlare_limits_defaults Resource limit defaults for tenants without overrides
# TYPE phlare_limits_defaults gauge
phlare_limits_defaults{limit_name="ingestion_rate_mb"} 20
phlare_limits_defaults{limit_name="ingestion_burst_size_mb"} 21
phlare_limits_defaults{limit_name="max_global_series_per_tenant"} 22
phlare_limits_defaults{limit_name="max_series_per_tenant"} 23
phlare_limits_defaults{limit_name="max_label_name_length"} 24
phlare_limits_defaults{limit_name="max_label_value_length"} 25
phlare_limits_defaults{limit_name="max_label_names_per_series"} 26
phlare_limits_defaults{limit_name="max_query_lookback"} 27
phlare_limits_defaults{limit_name="max_query_length"} 28
phlare_limits_defaults{limit_name="max_query_parallelism"} 29
`
	err = testutil.CollectAndCompare(exporter, bytes.NewBufferString(limitsMetrics), "phlare_limits_defaults")
	assert.NoError(t, err)
}

func TestOverridesExporter_withRing(t *testing.T) {
	tenantLimits := map[string]*validation.Limits{
		"tenant-a": {},
	}

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg1 := Config{RingConfig{}}
	cfg1.Ring.KVStore.Mock = ringStore
	cfg1.Ring.InstancePort = 1234
	cfg1.Ring.HeartbeatPeriod = 15 * time.Second
	cfg1.Ring.HeartbeatTimeout = 1 * time.Minute

	// Create an empty ring.
	ctx := context.Background()
	require.NoError(t, ringStore.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		return ring.NewDesc(), true, nil
	}))

	// Create an overrides-exporter.
	cfg1.Ring.InstanceID = "overrides-exporter-1"
	cfg1.Ring.InstanceAddr = "1.2.3.1"
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
	cfg2.Ring.InstanceID = "overrides-exporter-2"
	cfg2.Ring.InstanceAddr = "1.2.3.2"
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
	return testutil.CollectAndCount(e1, "phlare_limits_overrides") > 0
}
