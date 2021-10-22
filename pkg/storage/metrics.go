package storage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
)

type metrics struct {
	putTotal prometheus.Counter
	getTotal prometheus.Counter

	gcDuration prometheus.Summary
}

type cacheMetrics struct {
	cacheMisses *prometheus.CounterVec
	cacheReads  *prometheus.CounterVec

	cacheDBWrites *prometheus.HistogramVec
	cacheDBReads  *prometheus.HistogramVec

	evictionsDuration *prometheus.SummaryVec
	writeBackDuration *prometheus.SummaryVec
}

func newStorageMetrics(r prometheus.Registerer) *metrics {
	return &metrics{
		putTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_storage_writes_total",
			Help: "number of calls to storage.Put",
		}),
		getTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_storage_reads_total",
			Help: "number of calls to storage.Get",
		}),
		gcDuration: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_retention_duration_seconds",
			Help:       "duration of old data deletion",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
	}
}

func newCacheMetrics(r prometheus.Registerer) *cacheMetrics {
	name := []string{"name"}
	return &cacheMetrics{
		cacheMisses: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_cache_misses_total",
			Help: "total number of cache misses",
		}, name),
		cacheReads: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_cache_reads_total",
			Help: "total number of cache reads",
		}, name),

		cacheDBWrites: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_cache_db_write_bytes",
			Help:    "bytes written to db from cache",
			Buckets: []float64{1 << 10, 1 << 12, 1 << 14, 1 << 16, 1 << 18, 1 << 20, 1 << 22, 1 << 24, 1 << 26},
		}, name),
		cacheDBReads: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_cache_db_read_bytes",
			Help:    "bytes read from db to cache",
			Buckets: []float64{1 << 10, 1 << 12, 1 << 14, 1 << 16, 1 << 18, 1 << 20, 1 << 22, 1 << 24, 1 << 26},
		}, name),

		evictionsDuration: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_cache_evictions_duration_seconds",
			Help:       "duration of evictions (triggered when there's memory pressure)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, name),
		writeBackDuration: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_cache_writeback_duration_seconds",
			Help:       "duration of write-back writes (triggered periodically)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, name),
	}
}

func (m cacheMetrics) createInstance(name string) cache.Metrics {
	l := prometheus.Labels{"name": name}
	return cache.Metrics{
		MissesCounter:     m.cacheMisses.With(l),
		ReadsCounter:      m.cacheReads.With(l),
		DBWrites:          m.cacheDBWrites.With(l),
		DBReads:           m.cacheDBReads.With(l),
		EvictionsDuration: m.writeBackDuration.With(l),
		WriteBackDuration: m.evictionsDuration.With(l),
	}
}
