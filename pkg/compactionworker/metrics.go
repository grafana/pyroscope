package compactionworker

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type workerMetrics struct {
	jobsInProgress            *prometheus.GaugeVec
	jobsCompleted             *prometheus.CounterVec
	jobDuration               *prometheus.HistogramVec
	timeToCompaction          *prometheus.HistogramVec
	blocksDeleted             *prometheus.CounterVec
	symbolBloomBlockSizeBytes *prometheus.HistogramVec
	symbolBloomIndexSizeBytes *prometheus.HistogramVec
	symbolBloomUniqueSymbols  *prometheus.HistogramVec
	symbolBloomRows           *prometheus.HistogramVec
	symbolBloomBitsBytes      *prometheus.HistogramVec
}

func newMetrics(r prometheus.Registerer) *workerMetrics {
	m := &workerMetrics{
		jobsInProgress: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "jobs_in_progress",
			Help: "The number of active compaction jobs currently running.",
		}, []string{"tenant", "level"}),

		jobsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "jobs_completed_total",
			Help: "Total number of compaction jobs completed.",
		}, []string{"tenant", "level", "status"}),

		jobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "job_duration_seconds",
			Help: "Duration of compaction job runs",

			Buckets:                         prometheus.ExponentialBucketsRange(1, 300, 16),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"tenant", "level", "status"}),

		timeToCompaction: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "time_to_compaction_seconds",
			Help: "The time elapsed since the oldest compacted block was created.",

			Buckets:                         prometheus.ExponentialBucketsRange(1, 14400, 16),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"tenant", "level"}),

		blocksDeleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "blocks_deleted_total",
			Help: "Total number of block deletion attempts.",
		}, []string{"status"}),

		symbolBloomBlockSizeBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "symbol_bloom_block_size_bytes",
			Help:    "Size of compacted output blocks that contain a symbol Bloom index.",
			Buckets: prometheus.ExponentialBuckets(16<<10, 2, 16),
		}, []string{"tenant", "level"}),

		symbolBloomIndexSizeBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "symbol_bloom_index_size_bytes",
			Help:    "Size of the symbol Bloom index payload written to compacted output blocks.",
			Buckets: prometheus.ExponentialBuckets(1<<10, 2, 16),
		}, []string{"tenant", "level"}),

		symbolBloomUniqueSymbols: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "symbol_bloom_unique_symbols",
			Help:    "Estimated unique Function.Name values written to symbol Bloom indexes per compacted output block.",
			Buckets: prometheus.ExponentialBuckets(100, 2, 16),
		}, []string{"tenant", "level"}),

		symbolBloomRows: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "symbol_bloom_rows",
			Help:    "Number of service/profile rows written to symbol Bloom indexes per compacted output block.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}, []string{"tenant", "level"}),

		symbolBloomBitsBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "symbol_bloom_bits_bytes",
			Help:    "Raw Bloom bitset bytes written to symbol Bloom indexes per compacted output block.",
			Buckets: prometheus.ExponentialBuckets(512, 2, 16),
		}, []string{"tenant", "level"}),
	}

	util.Register(r,
		m.jobsInProgress,
		m.jobsCompleted,
		m.jobDuration,
		m.timeToCompaction,
		m.blocksDeleted,
		m.symbolBloomBlockSizeBytes,
		m.symbolBloomIndexSizeBytes,
		m.symbolBloomUniqueSymbols,
		m.symbolBloomRows,
		m.symbolBloomBitsBytes,
	)

	return m
}

func (m *workerMetrics) symbolBloomMetrics() *block.SymbolBloomMetrics {
	if m == nil {
		return nil
	}
	labels := func(tenant string, level uint32) []string {
		return []string{tenant, strconv.Itoa(int(level))}
	}
	return &block.SymbolBloomMetrics{
		BlockSizeBytes: func(tenant string, level uint32, bytes uint64) {
			m.symbolBloomBlockSizeBytes.WithLabelValues(labels(tenant, level)...).Observe(float64(bytes))
		},
		IndexSizeBytes: func(tenant string, level uint32, bytes uint64) {
			m.symbolBloomIndexSizeBytes.WithLabelValues(labels(tenant, level)...).Observe(float64(bytes))
		},
		UniqueSymbols: func(tenant string, level uint32, symbols uint64) {
			m.symbolBloomUniqueSymbols.WithLabelValues(labels(tenant, level)...).Observe(float64(symbols))
		},
		Rows: func(tenant string, level uint32, rows uint64) {
			m.symbolBloomRows.WithLabelValues(labels(tenant, level)...).Observe(float64(rows))
		},
		BloomBitsBytes: func(tenant string, level uint32, bytes uint64) {
			m.symbolBloomBitsBytes.WithLabelValues(labels(tenant, level)...).Observe(float64(bytes))
		},
	}
}
