package storage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
)

type metrics struct {
	writesTotal prometheus.Counter
	readsTotal  prometheus.Counter

	retentionTimer   prometheus.Histogram
	badgerCloseTimer prometheus.Histogram

	// TODO: I think we should make metrics below db-specific.
	//  Perhaps, we could even move those to cache metrics.

	evictionsTimer      prometheus.Histogram
	evictionsAllocBytes prometheus.Gauge
	evictionsTotalBytes prometheus.Gauge

	writeBackTimer prometheus.Histogram

	cacheFlushTimer prometheus.Histogram
}

type cacheMetrics struct {
	missCounterMetrics        *prometheus.CounterVec
	storageReadCounterMetrics *prometheus.CounterVec
	diskWritesCounterMetrics  *prometheus.HistogramVec
	diskReadsCounterMetrics   *prometheus.HistogramVec
}

func newStorageMetrics(r prometheus.Registerer) *metrics {
	return &metrics{
		writesTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_storage_writes_total",
			Help: "number of calls to storage.Put",
		}),
		readsTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_storage_reads_total",
			Help: "number of calls to storage.Get",
		}),

		// Evictions
		evictionsTimer: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_evictions_duration_seconds",
			Help:    "duration of evictions (triggered when there's memory pressure)",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
		// The following 2 metrics are somewhat broad
		// Nevertheless they are still useful to grasp evictions
		evictionsAllocBytes: promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_storage_evictions_alloc_bytes",
			Help: "number of bytes allocated in the heap",
		}),
		evictionsTotalBytes: promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_storage_evictions_total_mem_bytes",
			Help: "total number of memory bytes",
		}),

		cacheFlushTimer: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_caches_flush_duration_seconds",
			Help:    "duration of storage caches flush (triggered when server is closing)",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
		badgerCloseTimer: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_db_close_duration_seconds",
			Help:    "duration of db close (triggered when server is closing)",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),

		writeBackTimer: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_writeback_duration_seconds",
			Help:    "duration of write-back writes (triggered periodically)",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
		retentionTimer: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name: "pyroscope_storage_retention_duration_seconds",
			Help: "duration of old data deletion",
			// TODO what buckets to use here?
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
	}
}

func newCacheMetrics(r prometheus.Registerer) cacheMetrics {
	partitionBy := []string{"name"}
	return cacheMetrics{
		missCounterMetrics: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_cache_misses_total",
			Help: "total number of cache misses",
		}, partitionBy),
		storageReadCounterMetrics: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_storage_cache_reads_total",
			Help: "total number of cache queries",
		}, partitionBy),

		diskWritesCounterMetrics: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_cache_disk_writes",
			Help:    "items unloaded from cache to disk",
			Buckets: []float64{1 << 10, 1 << 12, 1 << 14, 1 << 16, 1 << 18, 1 << 20, 1 << 22, 1 << 24, 1 << 26},
		}, partitionBy),
		diskReadsCounterMetrics: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_storage_cache_disk_reads",
			Help:    "items loaded from disk to cache",
			Buckets: []float64{1 << 10, 1 << 12, 1 << 14, 1 << 16, 1 << 18, 1 << 20, 1 << 22, 1 << 24, 1 << 26},
		}, partitionBy),
	}
}

func (m cacheMetrics) createInstance(name string) cache.Metrics {
	l := prometheus.Labels{"name": name}
	return cache.Metrics{
		MissCounter:         m.missCounterMetrics.With(l),
		ReadCounter:         m.storageReadCounterMetrics.With(l),
		DiskWritesHistogram: m.diskWritesCounterMetrics.With(l),
		DiskReadsHistogram:  m.diskReadsCounterMetrics.With(l),
	}
}
