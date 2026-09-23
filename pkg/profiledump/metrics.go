package profiledump

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type recorderMetrics struct {
	candidates, bytes, dropped, uploads, uploadErrors *prometheus.CounterVec
	uploadDuration                                    *prometheus.HistogramVec
	retained, queueBytes, queueItems                  prometheus.Gauge
}

func newRecorderMetrics(reg prometheus.Registerer) recorderMetrics {
	f := promauto.With(reg)
	counter := func(name, help string, labels ...string) *prometheus.CounterVec {
		return f.NewCounterVec(prometheus.CounterOpts{Namespace: "pyroscope", Subsystem: "profile_dump", Name: name, Help: help}, labels)
	}
	gauge := func(name, help string) prometheus.Gauge {
		return f.NewGauge(prometheus.GaugeOpts{Namespace: "pyroscope", Subsystem: "profile_dump", Name: name, Help: help})
	}
	return recorderMetrics{
		candidates:     counter("candidates_total", "Capture admission outcomes; enqueue is not persistence.", "source", "result"),
		bytes:          counter("bytes_total", "Complete object bytes enqueued, dropped (including queued shutdown discards), or successfully uploaded; zero when sizing was not reached. Outcomes overlap.", "source", "result"),
		dropped:        counter("dropped_total", "Local capture drops by bounded reason, including queued shutdown discards; excludes upload failures.", "source", "reason"),
		uploads:        counter("uploads_total", "Completed background upload attempts by result: success, error, timeout, or canceled; excludes queued shutdown discards.", "source", "result"),
		uploadErrors:   counter("upload_errors_total", "Failed or timed out background uploads.", "source"),
		uploadDuration: f.NewHistogramVec(prometheus.HistogramOpts{Namespace: "pyroscope", Subsystem: "profile_dump", Name: "upload_duration_seconds", Help: "Background capture upload duration.", Buckets: prometheus.DefBuckets}, []string{"source"}),
		retained:       gauge("reserved_bytes", "Reserved bytes for preparation, queued and uploading captures, including scratch and encoding overhead."),
		queueBytes:     gauge("queue_bytes", "Complete object bytes waiting for an upload worker."),
		queueItems:     gauge("queue_items", "Captures waiting for an upload worker."),
	}
}

func metricSource(s SourceProtocol) string {
	switch s {
	case SourceConnect:
		return "connect"
	case SourceIngest:
		return "ingest"
	case SourceOTLPHTTP:
		return "otlp_http"
	case SourceOTLPGRPC:
		return "otlp_grpc"
	default:
		return "unknown"
	}
}

func (m recorderMetrics) admission(source string, o Outcome) {
	result := "dropped"
	if o.Enqueued {
		result = "enqueued"
	} else {
		m.dropped.WithLabelValues(source, string(o.Reason)).Inc()
	}
	m.candidates.WithLabelValues(source, result).Inc()
	m.bytes.WithLabelValues(source, result).Add(float64(o.Size))
}
