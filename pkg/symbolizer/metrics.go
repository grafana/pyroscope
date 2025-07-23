package symbolizer

import (
	"github.com/grafana/pyroscope/pkg/util"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Status values for metrics
	statusSuccess = "success"

	// Error status prefixes
	statusErrorPrefix = "error:"

	// HTTP error statuses
	statusErrorNotFound     = statusErrorPrefix + "not_found"
	statusErrorUnauthorized = statusErrorPrefix + "unauthorized"
	statusErrorRateLimited  = statusErrorPrefix + "rate_limited"
	statusErrorClientError  = statusErrorPrefix + "client_error"
	statusErrorServerError  = statusErrorPrefix + "server_error"
	statusErrorHTTPOther    = statusErrorPrefix + "http_other"

	// General error statuses
	statusErrorCanceled   = statusErrorPrefix + "canceled"
	statusErrorTimeout    = statusErrorPrefix + "timeout"
	statusErrorInvalidID  = statusErrorPrefix + "invalid_id"
	statusErrorOther      = statusErrorPrefix + "other"
	statusErrorDebuginfod = statusErrorPrefix + "debuginfod_error"
)

type metrics struct {
	registerer prometheus.Registerer

	// Debuginfod metrics
	debuginfodRequestDuration *prometheus.HistogramVec
	debuginfodFileSize        prometheus.Histogram

	// Cache metrics
	cacheOperations *prometheus.CounterVec
	cacheSizeBytes  *prometheus.GaugeVec

	// Profile symbolization metrics
	profileSymbolization *prometheus.HistogramVec

	// Debug symbol resolution metrics
	debugSymbolResolution       *prometheus.HistogramVec
	debugSymbolResolutionErrors *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		registerer: reg,
		debuginfodRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_symbolizer_debuginfod_request_duration_seconds",
			Help:    "Time spent performing debuginfod requests by status",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
		}, []string{"status"}),
		debuginfodFileSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name: "pyroscope_symbolizer_debuginfo_file_size_bytes",
				Help: "Size of debug info files fetched from debuginfod",
				// 1MB to 4GB
				Buckets: prometheus.ExponentialBuckets(1024*1024, 2, 12),
			},
		),
		// cache metrics
		cacheOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pyroscope_symbolizer_cache_operations_total",
				Help: "Total number of cache operations by cache type, operation and status",
			},
			[]string{"cache_type", "operation", "status"},
		),
		cacheSizeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_symbolizer_cache_size_bytes",
			Help: "Current size of cache in bytes by cache type",
		}, []string{"cache_type"}),
		// profile symbolization metrics
		profileSymbolization: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_profile_symbolization_duration_seconds",
			Help:    "Time spent performing profile symbolization by status",
			Buckets: []float64{.01, .05, .1, .5, 1, 5, 10, 30},
		}, []string{"status"}),
		// debug symbol resolution metrics
		debugSymbolResolution: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_debug_symbol_resolution_duration_seconds",
			Help:    "Time spent resolving debug symbols from ELF files by status",
			Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10},
		}, []string{"status"}),
		debugSymbolResolutionErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pyroscope_debug_symbol_resolution_errors_total",
				Help: "Total number of errors encountered during debug symbol resolution by error type",
			},
			[]string{"error_type"},
		),
	}

	if reg != nil {
		m.register()
	}

	return m
}

func (m *metrics) register() {
	if m.registerer == nil {
		return
	}

	collectors := []prometheus.Collector{
		m.debuginfodRequestDuration,
		m.debuginfodFileSize,
		m.cacheOperations,
		m.cacheSizeBytes,
		m.profileSymbolization,
		m.debugSymbolResolution,
		m.debugSymbolResolutionErrors,
	}

	for _, collector := range collectors {
		util.RegisterOrGet(m.registerer, collector)
	}
}
