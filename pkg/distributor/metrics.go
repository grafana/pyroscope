package distributor

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	minBytes     = 10 * 1024
	maxBytes     = 15 * 1024 * 1024
	bucketsCount = 30
)

type metrics struct {
	receivedCompressedBytes   *prometheus.HistogramVec
	receivedDecompressedBytes *prometheus.HistogramVec
	receivedSamples           *prometheus.HistogramVec
	receivedSamplesBytes      *prometheus.HistogramVec
	receivedSymbolsBytes      *prometheus.HistogramVec
	replicationFactor         prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		replicationFactor: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "phlare",
			Name:      "distributor_replication_factor",
			Help:      "The configured replication factor for the distributor.",
		}),
		receivedCompressedBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_compressed_bytes",
				Help:      "The number of compressed bytes per profile received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{"type", "tenant"},
		),
		receivedDecompressedBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_decompressed_bytes",
				Help:      "The number of decompressed bytes per profiles received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{"type", "tenant"},
		),
		receivedSamples: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_samples",
				Help:      "The number of samples per profile name received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(100, 100000, 30),
			},
			[]string{"type", "tenant"},
		),
		receivedSamplesBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_samples_bytes",
				Help:      "The size of samples without symbols received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"type", "tenant"},
		),
		receivedSymbolsBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "phlare",
				Name:      "distributor_received_symbols_bytes",
				Help:      "The size of symbols received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"type", "tenant"},
		),
	}
	if reg != nil {
		reg.MustRegister(
			m.receivedCompressedBytes,
			m.receivedDecompressedBytes,
			m.receivedSamples,
			m.receivedSamplesBytes,
			m.receivedSymbolsBytes,
		)
	}
	return m
}
