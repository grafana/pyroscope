package storage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
)

type metrics struct {
	putTotal prometheus.Counter
	getTotal prometheus.Counter

	retentionTaskDuration prometheus.Summary
	evictionTaskDuration  prometheus.Summary
	writeBackTaskDuration prometheus.Summary

	dbSize    *prometheus.GaugeVec
	cacheSize *prometheus.GaugeVec
	gcCount   *prometheus.CounterVec

	cacheMisses   *prometheus.CounterVec
	cacheReads    *prometheus.CounterVec
	cacheDBWrites *prometheus.HistogramVec
	cacheDBReads  *prometheus.HistogramVec

	evictionsDuration *prometheus.SummaryVec
	writeBackDuration *prometheus.SummaryVec

	exemplarsWriteBytes            prometheus.Summary
	exemplarsReadBytes             prometheus.Summary
	exemplarsRemovedTotal          prometheus.Counter
	exemplarsDiscardedTotal        prometheus.Counter
	exemplarsRetentionTaskDuration prometheus.Summary
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

		retentionTaskDuration: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_retention_task_duration_seconds",
			Help:       "duration of old data deletion",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		evictionTaskDuration: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_eviction_task_duration_seconds",
			Help:       "duration of evictions (triggered when there's memory pressure)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		writeBackTaskDuration: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_writeback_task_duration_seconds",
			Help:       "duration of write-back writes (triggered periodically)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),

		dbSize: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_storage_db_size_bytes",
			Help: "size of items in disk",
		}, name),
		gcCount: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_db_gc_total",
			Help: "number of GC runs",
		}, name),
		cacheSize: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_storage_db_cache_size",
			Help: "number of items in cache",
		}, name),

		cacheDBWrites: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_db_cache_write_bytes",
			Help:    "bytes written to db from cache",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		}, name),
		cacheDBReads: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_db_cache_read_bytes",
			Help:    "bytes read from db to cache",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		}, name),

		cacheMisses: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_db_cache_misses_total",
			Help: "total number of cache misses",
		}, name),
		cacheReads: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_db_cache_reads_total",
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

		exemplarsWriteBytes: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_exemplars_write_bytes",
			Help:       "bytes written to exemplars storage",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		exemplarsReadBytes: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_exemplars_read_bytes",
			Help:       "bytes read from exemplars storage",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		exemplarsRemovedTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_storage_exemplars_removed_total",
			Help: "number of exemplars removed from storage based on the retention policy",
		}),
		exemplarsDiscardedTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_storage_exemplars_discarded_total",
			Help: "number of exemplars discarded",
		}),
		exemplarsRetentionTaskDuration: promauto.With(r).NewSummary(prometheus.SummaryOpts{
			Name:       "pyroscope_storage_exemplars_retention_task_duration_seconds",
			Help:       "time taken to enforce exemplars retention policy",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
	}
}

func (m *metrics) createCacheMetrics(name string) *cache.Metrics {
	return &cache.Metrics{
		MissesCounter:     m.cacheMisses.WithLabelValues(name),
		ReadsCounter:      m.cacheReads.WithLabelValues(name),
		DBWrites:          m.cacheDBWrites.WithLabelValues(name),
		DBReads:           m.cacheDBReads.WithLabelValues(name),
		EvictionsDuration: m.evictionsDuration.WithLabelValues(name),
		WriteBackDuration: m.writeBackDuration.WithLabelValues(name),
	}
}
