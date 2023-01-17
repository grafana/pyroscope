package distributor

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	receivedCompressedBytes   *prometheus.HistogramVec
	receivedDecompressedBytes *prometheus.HistogramVec
	receivedSamples           *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		receivedCompressedBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_compressed_bytes",
				Help:      "The number of compressed bytes per profile received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"type"},
		),
		receivedDecompressedBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_decompressed_bytes",
				Help:      "The number of decompressed bytes per profiles received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"type"},
		),
		receivedSamples: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_samples",
				Help:      "The number of samples per profile name received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(100, 100000, 30),
			},
			[]string{"type"},
		),
	}
	if reg != nil {
		reg.MustRegister(
			m.receivedCompressedBytes,
			m.receivedDecompressedBytes,
			m.receivedSamples,
		)
	}
	return m
}
