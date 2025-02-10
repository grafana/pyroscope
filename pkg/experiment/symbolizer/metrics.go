package symbolizer

import (
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	registerer prometheus.Registerer

	// Debuginfod metrics
	debuginfodRequestDuration    *prometheus.HistogramVec
	debuginfodFileSize           prometheus.Histogram
	debuginfodRequestsTotal      prometheus.Counter
	debuginfodRequestErrorsTotal *prometheus.CounterVec

	// Cache metrics
	cacheRequestsTotal      *prometheus.CounterVec
	cacheRequestErrorsTotal *prometheus.CounterVec
	cacheHitsTotal          prometheus.Counter
	cacheMissesTotal        prometheus.Counter
	cacheOperationDuration  *prometheus.HistogramVec
	cacheExpiredTotal       prometheus.Counter

	// Symbolization tree-level metrics
	symbolizeRequestsTotal prometheus.Counter
	symbolizeErrorsTotal   *prometheus.CounterVec
	symbolizeDuration      prometheus.Histogram

	// Internal symbolization metrics
	symbolizeLocationTotal    *prometheus.CounterVec
	symbolizeInternalErrors   *prometheus.CounterVec
	symbolizeInternalDuration prometheus.Histogram
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		registerer: reg,
		debuginfodRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_symbolizer_debuginfod_request_duration_seconds",
			Help:    "Time spent performing debuginfod requests",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
		}, []string{"status"},
		),
		debuginfodFileSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name: "pyroscope_symbolizer_debuginfo_file_size_bytes",
				Help: "Size of debug info files fetched from debuginfod",
				// 1MB to 4GB
				Buckets: prometheus.ExponentialBuckets(1024*1024, 2, 12),
			},
		),
		debuginfodRequestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_debuginfod_requests_total",
			Help: "Total number of debuginfod requests attempted",
		}),
		debuginfodRequestErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_debuginfod_request_errors_total",
			Help: "Total number of debuginfod request errors",
		}, []string{"reason"}),
		cacheRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_cache_requests_total",
			Help: "Total number of cache requests",
		}, []string{"operation"}),
		cacheRequestErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_cache_request_errors_total",
			Help: "Total number of cache request errors",
		}, []string{"operation", "reason"}), // get/put, and specific error reasons
		cacheHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_cache_hits_total",
			Help: "Total number of cache hits",
		}),
		cacheMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_cache_misses_total",
			Help: "Total number of cache misses",
		}),
		cacheOperationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pyroscope_symbolizer_cache_operation_duration_seconds",
				Help:    "Time spent performing cache operations",
				Buckets: []float64{.01, .05, .1, .5, 1, 5, 10, 30, 60},
			},
			[]string{"operation"},
		),
		cacheExpiredTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_cache_expired_total",
			Help: "Total number of expired items removed from cache",
		}),
		symbolizeRequestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_tree_requests_total",
			Help: "Total number of tree symbolization requests",
		}),
		symbolizeErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_tree_errors_total",
			Help: "Total number of tree symbolization errors",
		}, []string{"reason"}),
		symbolizeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "pyroscope_symbolizer_tree_duration_seconds",
			Help:    "Time spent performing tree symbolization",
			Buckets: []float64{.01, .05, .1, .5, 1, 5, 10, 30},
		}),
		symbolizeLocationTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_locations_total",
			Help: "Total number of locations processed",
		}, []string{"status"}),
		symbolizeInternalErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symbolizer_internal_errors_total",
			Help: "Total number of internal symbolization errors",
		}, []string{"reason"}),
		symbolizeInternalDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "pyroscope_symbolizer_internal_duration_seconds",
			Help:    "Time spent performing internal symbolization operations",
			Buckets: []float64{.01, .05, .1, .5, 1, 5, 10, 30},
		}),
	}

	if reg != nil {
		m.register()
	}

	return m
}

func (m *Metrics) register() {
	if m.registerer == nil {
		return
	}

	collectors := []prometheus.Collector{
		m.debuginfodRequestDuration,
		m.debuginfodFileSize,
		m.debuginfodRequestErrorsTotal,
		m.debuginfodRequestsTotal,
		m.cacheRequestsTotal,
		m.cacheRequestErrorsTotal,
		m.cacheHitsTotal,
		m.cacheMissesTotal,
		m.cacheOperationDuration,
		m.cacheExpiredTotal,
		m.symbolizeRequestsTotal,
		m.symbolizeErrorsTotal,
		m.symbolizeDuration,
		m.symbolizeLocationTotal,
		m.symbolizeInternalErrors,
		m.symbolizeInternalDuration,
	}

	for _, collector := range collectors {
		util.RegisterOrGet(m.registerer, collector)
	}
}

func (m *Metrics) Unregister() {
	if m.registerer == nil {
		return
	}

	collectors := []prometheus.Collector{
		m.debuginfodRequestDuration,
		m.debuginfodFileSize,
		m.debuginfodRequestErrorsTotal,
		m.debuginfodRequestsTotal,
		m.cacheRequestsTotal,
		m.cacheRequestErrorsTotal,
		m.cacheHitsTotal,
		m.cacheMissesTotal,
		m.cacheOperationDuration,
		m.cacheExpiredTotal,
		m.symbolizeRequestsTotal,
		m.symbolizeErrorsTotal,
		m.symbolizeDuration,
		m.symbolizeLocationTotal,
		m.symbolizeInternalErrors,
		m.symbolizeInternalDuration,
	}

	for _, collector := range collectors {
		m.registerer.Unregister(collector)
	}
}
