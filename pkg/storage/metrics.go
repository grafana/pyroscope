package storage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
)

type metrics struct {
	putTotal   prometheus.Counter
	getTotal   prometheus.Counter
	gcDuration prometheus.Summary

	dbSize    *prometheus.GaugeVec
	cacheSize *prometheus.GaugeVec

	cacheMisses   *prometheus.CounterVec
	cacheReads    *prometheus.CounterVec
	cacheDBWrites *prometheus.HistogramVec
	cacheDBReads  *prometheus.HistogramVec

	evictionsDuration *prometheus.SummaryVec
	writeBackDuration *prometheus.SummaryVec
}

func newMetrics(r prometheus.Registerer) *metrics {
	name := []string{"name"}
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
			Name:       "pyroscope_storage_gc_duration_seconds",
			Help:       "duration of old data deletion",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),

		dbSize: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_storage_db_size_bytes",
			Help: "size of items in disk",
		}, name),
		cacheSize: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_storage_cache_size",
			Help: "number of items in cache",
		}, name),

		cacheDBWrites: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_cache_db_write_bytes",
			Help:    "bytes written to db from cache",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		}, name),
		cacheDBReads: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_cache_db_read_bytes",
			Help:    "bytes read from db to cache",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		}, name),

		cacheMisses: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_cache_misses_total",
			Help: "total number of cache misses",
		}, name),
		cacheReads: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_cache_reads_total",
			Help: "total number of cache reads",
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

func (m *metrics) createCacheMetrics(name string) *cache.Metrics {
	l := prometheus.Labels{"name": name}
	return &cache.Metrics{
		MissesCounter:     m.cacheMisses.With(l),
		ReadsCounter:      m.cacheReads.With(l),
		DBWrites:          m.cacheDBWrites.With(l),
		DBReads:           m.cacheDBReads.With(l),
		EvictionsDuration: m.evictionsDuration.With(l),
		WriteBackDuration: m.writeBackDuration.With(l),
	}
}
