package parser

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
)

var (
	requestsTotalCounter *prometheus.CounterVec
	requestsBytesCounter *prometheus.CounterVec
)

func init() {
	requestsTotalCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pyroscope_parser_incoming_requests_total",
			Help: "Total number of requests received by the parser",
		},
		[]string{"profiler"},
	)
	requestsBytesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pyroscope_parser_incoming_requests_bytes",
			Help: "Total number of bytes received by the parser",
		},
		[]string{"profiler"},
	)

	prometheus.MustRegister(requestsTotalCounter, requestsBytesCounter)
}

func updateMetrics(in *ingestion.IngestInput) {
	profilerName := "unknown"
	if in.Metadata.SpyName != "" {
		profilerName = in.Metadata.SpyName
	}
	requestsTotalCounter.WithLabelValues(profilerName).Inc()

	data, err := in.Profile.Bytes()
	if err == nil {
		requestsBytesCounter.WithLabelValues(profilerName).Add(float64(len(data)))
	}
}
