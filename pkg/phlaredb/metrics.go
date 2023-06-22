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
	samples              prometheus.Gauge

	flushedFileSizeBytes        *prometheus.HistogramVec
	flushedBlockSizeBytes       prometheus.Histogram
	flushedBlockDurationSeconds prometheus.Histogram
	flushedBlockSeries          prometheus.Histogram
	flushedBlockSamples         prometheus.Histogram
	flusehdBlockProfiles        prometheus.Histogram
	blockDurationSeconds        prometheus.Histogram
	flushedBlocks               *prometheus.CounterVec
	flushedBlocksReasons        *prometheus.CounterVec
	writtenProfileSegments      *prometheus.CounterVec
	writtenProfileSegmentsBytes prometheus.Histogram
}

func newHeadMetrics(reg prometheus.Registerer) *headMetrics {
	m := &headMetrics{
		seriesCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_tsdb_head_series_created_total",
			Help: "Total number of series created in the head",
		}, []string{"profile_name"}),
		rowsWritten: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pyroscope_rows_written",
				Help: "Number of rows written to a parquet table.",
			},
			[]string{"type"}),
		profilesCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_head_profiles_created_total",
			Help: "Total number of profiles created in the head",
		}, []string{"profile_name"}),
		sampleValuesIngested: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pyroscope_head_ingested_sample_values_total",
				Help: "Number of sample values ingested into the head per profile type.",
			},
			[]string{"profile_name"}),
		sampleValuesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pyroscope_head_received_sample_values_total",
				Help: "Number of sample values received into the head per profile type.",
			},
			[]string{"profile_name"}),
		sizeBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pyroscope_head_size_bytes",
				Help: "Size of a particular in memory store within the head phlaredb block.",
			},
			[]string{"type"}),
		series: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_tsdb_head_series",
			Help: "Total number of series in the head block.",
		}),
		profiles: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_head_profiles",
			Help: "Total number of profiles in the head block.",
		}),
		flushedFileSizeBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "pyroscope_head_flushed_table_size_bytes",
			Help: "Size of a flushed table in bytes.",
			//  [2MB, 4MB, 8MB, 16MB, 32MB, 64MB, 128MB, 256MB, 512MB, 1GB, 2GB]
			Buckets: prometheus.ExponentialBuckets(2*1024*1024, 2, 11),
		}, []string{"name"}),
		flushedBlockSizeBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_flushed_block_size_bytes",
			Help: "Size of a flushed block in bytes.",
			// [50MB, 75MB, 112.5MB, 168.75MB, 253.125MB, 379.688MB, 569.532MB, 854.298MB, 1.281MB, 1.922MB, 2.883MB]
			Buckets: prometheus.ExponentialBuckets(50*1024*1024, 1.5, 11),
		}),
		flushedBlockDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_flushed_block_duration_seconds",
			Help: "Time to flushed a block in seconds.",
			// [5s, 7.5s, 11.25s, 16.875s, 25.3125s, 37.96875s, 56.953125s, 85.4296875s, 128.14453125s, 192.216796875s]
			Buckets: prometheus.ExponentialBuckets(5, 1.5, 10),
		}),
		flushedBlockSeries: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_flushed_block_series",
			Help: "Number of series in a flushed block.",
			// [1k, 3k, 5k, 7k, 9k, 11k, 13k, 15k, 17k, 19k]
			Buckets: prometheus.LinearBuckets(1000, 2000, 10),
		}),
		flushedBlockSamples: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_flushed_block_samples",
			Help: "Number of samples in a flushed block.",
			// [200k, 400k, 800k, 1.6M, 3.2M, 6.4M, 12.8M, 25.6M, 51.2M, 102.4M, 204.8M, 409.6M, 819.2M, 1.6384G, 3.2768G]
			Buckets: prometheus.ExponentialBuckets(200_000, 2, 15),
		}),
		flusehdBlockProfiles: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_flushed_block_profiles",
			Help: "Number of profiles in a flushed block.",
			// [20k, 40k, 80k, 160k, 320k, 640k, 1.28M, 2.56M, 5.12M, 10.24M]
			Buckets: prometheus.ExponentialBuckets(20_000, 2, 10),
		}),
		blockDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_block_duration_seconds",
			Help: "Duration of a block in seconds (the range it covers).",
			// [20m, 40m, 1h, 1h20, 1h40, 2h, 2h20, 2h40, 3h, 3h20, 3h40, 4h, 4h20, 4h40, 5h, 5h20, 5h40, 6h, 6h20, 6h40, 7h, 7h20, 7h40, 8h]
			Buckets: prometheus.LinearBuckets(1200, 1200, 24),
		}),
		flushedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_head_flushed_blocks_total",
			Help: "Total number of blocks flushed.",
		}, []string{"status"}),
		flushedBlocksReasons: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_head_flushed_reason_total",
			Help: "Total count of reasons why block has been flushed.",
		}, []string{"reason"}),
		writtenProfileSegments: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_head_written_profile_segments_total",
			Help: "Total number and status of profile row groups segments written.",
		}, []string{"status"}),
		writtenProfileSegmentsBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_head_written_profile_segments_size_bytes",
			Help: "Size of a flushed table in bytes.",
			//  [512KB, 1MB, 2MB, 4MB, 8MB, 16MB, 32MB, 64MB, 128MB, 256MB, 512MB]
			Buckets: prometheus.ExponentialBuckets(512*1024, 2, 11),
		}),
		samples: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_head_samples",
			Help: "Number of samples in the head.",
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
	m.flushedFileSizeBytes = util.RegisterOrGet(reg, m.flushedFileSizeBytes)
	m.flushedBlockSizeBytes = util.RegisterOrGet(reg, m.flushedBlockSizeBytes)
	m.flushedBlockDurationSeconds = util.RegisterOrGet(reg, m.flushedBlockDurationSeconds)
	m.flushedBlockSeries = util.RegisterOrGet(reg, m.flushedBlockSeries)
	m.flushedBlockSamples = util.RegisterOrGet(reg, m.flushedBlockSamples)
	m.flusehdBlockProfiles = util.RegisterOrGet(reg, m.flusehdBlockProfiles)
	m.blockDurationSeconds = util.RegisterOrGet(reg, m.blockDurationSeconds)
	m.flushedBlocks = util.RegisterOrGet(reg, m.flushedBlocks)
	m.flushedBlocksReasons = util.RegisterOrGet(reg, m.flushedBlocksReasons)
	m.writtenProfileSegments = util.RegisterOrGet(reg, m.writtenProfileSegments)
	m.writtenProfileSegmentsBytes = util.RegisterOrGet(reg, m.writtenProfileSegmentsBytes)
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
	blockOpened         prometheus.Gauge
}

func newBlocksMetrics(reg prometheus.Registerer) *blocksMetrics {
	m := &blocksMetrics{
		query: query.NewMetrics(reg),
		blockOpeningLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscopedb_block_opening_duration",
			Help: "Latency of opening a block in seconds",
		}),
		blockOpened: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscopedb_blocks_currently_open",
			Help: "Number of blocks opened",
		}),
	}
	m.blockOpeningLatency = util.RegisterOrGet(reg, m.blockOpeningLatency)
	m.blockOpened = util.RegisterOrGet(reg, m.blockOpened)
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
