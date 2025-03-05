package symbolizer

import (
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Status values for metrics
	StatusSuccess   = "success"
	StatusCacheHit  = "cache_hit"
	StatusCacheMiss = "miss"

	// Error status prefixes
	StatusErrorPrefix = "error:"

	// HTTP error statuses
	StatusErrorNotFound     = StatusErrorPrefix + "not_found"
	StatusErrorUnauthorized = StatusErrorPrefix + "unauthorized"
	StatusErrorRateLimited  = StatusErrorPrefix + "rate_limited"
	StatusErrorClientError  = StatusErrorPrefix + "client_error"
	StatusErrorServerError  = StatusErrorPrefix + "server_error"
	StatusErrorHTTPOther    = StatusErrorPrefix + "http_other"

	// General error statuses
	StatusErrorCanceled   = StatusErrorPrefix + "canceled"
	StatusErrorTimeout    = StatusErrorPrefix + "timeout"
	StatusErrorInvalidID  = StatusErrorPrefix + "invalid_id"
	StatusErrorOther      = StatusErrorPrefix + "other"
	StatusErrorRead       = StatusErrorPrefix + "read_error"
	StatusErrorUpload     = StatusErrorPrefix + "upload_error"
	StatusErrorDebuginfod = StatusErrorPrefix + "debuginfod_error"
	StatusErrorResolve    = StatusErrorPrefix + "resolve_error"
)

type Metrics struct {
	registerer prometheus.Registerer

	// Debuginfod metrics
	debuginfodRequestDuration *prometheus.HistogramVec
	debuginfodFileSize        prometheus.Histogram

	// Cache metrics
	cacheOperations *prometheus.HistogramVec
	cacheSizeBytes  *prometheus.GaugeVec

	// Profile symbolization metrics
	profileSymbolization *prometheus.HistogramVec

	// Debug symbol resolution metrics
	debugSymbolResolution *prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
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
		cacheOperations: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pyroscope_symbolizer_cache_operation_duration_seconds",
				Help:    "Time spent performing cache operations by cache type, operation and status",
				Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10},
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
		m.cacheOperations,
		m.cacheSizeBytes,
		m.profileSymbolization,
		m.debugSymbolResolution,
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
		m.cacheOperations,
		m.cacheSizeBytes,
		m.profileSymbolization,
		m.debugSymbolResolution,
	}

	for _, collector := range collectors {
		m.registerer.Unregister(collector)
	}
}
