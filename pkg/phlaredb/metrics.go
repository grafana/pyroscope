package phlaredb

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/query"
	"github.com/grafana/phlare/pkg/util"
)

type contextKey uint8

const (
	headMetricsContextKey contextKey = iota
	blockMetricsContextKey
)

type headMetrics struct {
	series        prometheus.Gauge
	seriesCreated *prometheus.CounterVec

	profiles        prometheus.Gauge
	profilesCreated *prometheus.CounterVec

	sizeBytes   *prometheus.GaugeVec
	rowsWritten *prometheus.CounterVec

	sampleValuesIngested *prometheus.CounterVec
	sampleValuesReceived *prometheus.CounterVec
}

func newHeadMetrics(reg prometheus.Registerer) *headMetrics {
	m := &headMetrics{
		seriesCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "phlare_tsdb_head_series_created_total",
			Help: "Total number of series created in the head",
		}, []string{"profile_name"}),
		rowsWritten: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "phlare_rows_written",
				Help: "Number of rows written to a parquet table.",
			},
			[]string{"type"}),
		profilesCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "phlare_head_profiles_created_total",
			Help: "Total number of profiles created in the head",
		}, []string{"profile_name"}),
		sampleValuesIngested: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "phlare_head_ingested_sample_values_total",
				Help: "Number of sample values ingested into the head per profile type.",
			},
			[]string{"profile_name"}),
		sampleValuesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "phlare_head_received_sample_values_total",
				Help: "Number of sample values received into the head per profile type.",
			},
			[]string{"profile_name"}),
		sizeBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "phlare_head_size_bytes",
				Help: "Size of a particular in memory store within the head phlaredb block.",
			},
			[]string{"type"}),
		series: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "phlare_tsdb_head_series",
			Help: "Total number of series in the head block.",
		}),
		profiles: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "phlare_head_profiles",
			Help: "Total number of profiles in the head block.",
		}),
	}

	m.register(reg)
	return m
}

func (m *headMetrics) register(reg prometheus.Registerer) {
	if reg == nil {
		return
	}
	m.series = util.RegisterOrGet(reg, m.series)
	m.seriesCreated = util.RegisterOrGet(reg, m.seriesCreated)
	m.profiles = util.RegisterOrGet(reg, m.profiles)
	m.profilesCreated = util.RegisterOrGet(reg, m.profilesCreated)
	m.sizeBytes = util.RegisterOrGet(reg, m.sizeBytes)
	m.rowsWritten = util.RegisterOrGet(reg, m.rowsWritten)
	m.sampleValuesIngested = util.RegisterOrGet(reg, m.sampleValuesIngested)
	m.sampleValuesReceived = util.RegisterOrGet(reg, m.sampleValuesReceived)
}

func contextWithHeadMetrics(ctx context.Context, m *headMetrics) context.Context {
	return context.WithValue(ctx, headMetricsContextKey, m)
}

func contextHeadMetrics(ctx context.Context) *headMetrics {
	m, ok := ctx.Value(headMetricsContextKey).(*headMetrics)
	if !ok {
		return newHeadMetrics(phlarecontext.Registry(ctx))
	}
	return m
}

type blocksMetrics struct {
	query *query.Metrics

	blockOpeningLatency prometheus.Histogram
}

func newBlocksMetrics(reg prometheus.Registerer) *blocksMetrics {
	m := &blocksMetrics{
		query: query.NewMetrics(reg),
		blockOpeningLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlaredb_block_opening_duration",
			Help: "Latency of opening a block in seconds",
		}),
	}
	m.blockOpeningLatency = util.RegisterOrGet(reg, m.blockOpeningLatency)
	return m
}

func contextWithBlockMetrics(ctx context.Context, m *blocksMetrics) context.Context {
	return context.WithValue(ctx, blockMetricsContextKey, m)
}

func contextBlockMetrics(ctx context.Context) *blocksMetrics {
	m, ok := ctx.Value(blockMetricsContextKey).(*blocksMetrics)
	if !ok {
		return newBlocksMetrics(phlarecontext.Registry(ctx))
	}
	return m
}
