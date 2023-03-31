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

	flushedFileSizeBytes        *prometheus.HistogramVec
	flushedBlockSizeBytes       prometheus.Histogram
	flushedBlockDurationSeconds prometheus.Histogram
	flushedBlockSeries          prometheus.Histogram
	flushedBlockSamples         prometheus.Histogram
	flusehdBlockProfiles        prometheus.Histogram
	blockDurationSeconds        prometheus.Histogram
	flushedBlocks               *prometheus.CounterVec
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
		flushedFileSizeBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "phlare_head_flushed_table_size_bytes",
			Help: "Size of a flushed table in bytes.",
			//  [2MB, 4MB, 8MB, 16MB, 32MB, 64MB, 128MB, 256MB, 512MB, 1GB, 2GB]
			Buckets: prometheus.ExponentialBuckets(2*1024*1024, 2, 11),
		}, []string{"name"}),
		flushedBlockSizeBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlare_head_flushed_block_size_bytes",
			Help: "Size of a flushed block in bytes.",
			// [50MB, 75MB, 112.5MB, 168.75MB, 253.125MB, 379.688MB, 569.532MB, 854.298MB, 1.281MB, 1.922MB, 2.883MB]
			Buckets: prometheus.ExponentialBuckets(50*1024*1024, 1.5, 11),
		}),
		flushedBlockDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlare_head_flushed_block_duration_seconds",
			Help: "Time to flushed a block in seconds.",
			// [100ms, 200ms, 400ms, 800ms, 1.6s, 3.2s, 6.4s, 12.8s, 25.6s, 51.2s]
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		}),
		flushedBlockSeries: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlare_head_flushed_block_series",
			Help: "Number of series in a flushed block.",
			// [5k, 10k, 20k, 40k, 80k, 160k, 320k, 640k, 1.28M, 2.56M]
			Buckets: prometheus.LinearBuckets(5000, 5000, 10),
		}),
		flushedBlockSamples: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlare_head_flushed_block_samples",
			Help: "Number of samples in a flushed block.",
			// [10k, 20k, 40k, 80k, 160k, 320k, 640k, 1.28M, 2.56M, 5.12M]
			Buckets: prometheus.ExponentialBuckets(10000, 2, 10),
		}),
		flusehdBlockProfiles: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlare_head_flushed_block_profiles",
			Help: "Number of profiles in a flushed block.",
			// [1k, 2k, 4k, 8k, 16k, 32k, 64k, 128k, 256k, 512k]
			Buckets: prometheus.ExponentialBuckets(1000, 2, 10),
		}),
		blockDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "phlare_head_block_duration_seconds",
			Help: "Duration of a block in seconds (the range it covers).",
			// [20m, 40m, 1h, 1h20, 1h40, 2h, 2h20, 2h40, 3h, 3h20, 3h40, 4h, 4h20, 4h40, 5h, 5h20, 5h40, 6h, 6h20, 6h40, 7h, 7h20, 7h40, 8h]
			Buckets: prometheus.LinearBuckets(1200, 1200, 24),
		}),
		flushedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "phlare_head_flushed_blocks_total",
			Help: "Total number of blocks flushed.",
		}, []string{"status"}),
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
	m.flushedFileSizeBytes = util.RegisterOrGet(reg, m.flushedFileSizeBytes)
	m.flushedBlockSizeBytes = util.RegisterOrGet(reg, m.flushedBlockSizeBytes)
	m.flushedBlockDurationSeconds = util.RegisterOrGet(reg, m.flushedBlockDurationSeconds)
	m.flushedBlockSeries = util.RegisterOrGet(reg, m.flushedBlockSeries)
	m.flushedBlockSamples = util.RegisterOrGet(reg, m.flushedBlockSamples)
	m.flusehdBlockProfiles = util.RegisterOrGet(reg, m.flusehdBlockProfiles)
	m.blockDurationSeconds = util.RegisterOrGet(reg, m.blockDurationSeconds)
	m.flushedBlocks = util.RegisterOrGet(reg, m.flushedBlocks)
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
