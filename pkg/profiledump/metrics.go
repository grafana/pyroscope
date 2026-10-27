package profiledump

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type recorderMetrics struct {
	captures, uploads *prometheus.CounterVec
}

func newRecorderMetrics(reg prometheus.Registerer) recorderMetrics {
	f := promauto.With(reg)
	counter := func(name, help string) *prometheus.CounterVec {
		return f.NewCounterVec(prometheus.CounterOpts{Namespace: "pyroscope", Subsystem: "profile_dump", Name: name, Help: help}, []string{"result"})
	}
	return recorderMetrics{
		captures: counter("captures_total", "Profile sample admission decisions by result. Enqueued samples are accepted for asynchronous upload, not yet persisted."),
		uploads:  counter("uploads_total", "Completed capture upload attempts by result, excluding shutdown interruptions. Success requires both payload and metadata uploads."),
	}
}

func (m recorderMetrics) admission(reason dropReason) {
	result := string(reason)
	if reason == "" {
		result = "enqueued"
	}
	m.captures.WithLabelValues(result).Inc()
}
