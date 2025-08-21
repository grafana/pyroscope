package distributor

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	minBytes     = 10 * 1024
	maxBytes     = 15 * 1024 * 1024
	bucketsCount = 30
)

type ReceiveStage string

const (
	// StageReceived is the earliest stage and as soon as we begin processing a profile,
	// before any rate-limit/sampling checks
	StageReceived ReceiveStage = "received"
	// StageSampled is recorded after the profile is accepted by rate-limit/sampling checks
	StageSampled ReceiveStage = "sampled"
	// StageNormalized is recorded after the profile is validated and normalized.
	StageNormalized ReceiveStage = "normalized"
)

var allStages = fmt.Sprintf("%s, %s, %s",
	StageReceived,
	StageSampled,
	StageNormalized,
)

type metrics struct {
	receivedCompressedBytes        *prometheus.HistogramVec
	receivedDecompressedBytes      *prometheus.HistogramVec // deprecated TODO remove
	receivedSamples                *prometheus.HistogramVec
	receivedSamplesBytes           *prometheus.HistogramVec
	receivedSymbolsBytes           *prometheus.HistogramVec
	replicationFactor              prometheus.Gauge
	receivedDecompressedBytesTotal *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		replicationFactor: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Name:      "distributor_replication_factor",
			Help:      "The configured replication factor for the distributor.",
		}),
		receivedCompressedBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_compressed_bytes",
				Help:      "The number of compressed bytes per profile received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{"type", "tenant"},
		),
		receivedDecompressedBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_decompressed_bytes",
				Help: "The number of decompressed bytes per profiles received by the distributor after " +
					"limits/sampling checks. distributor_received_decompressed_bytes is deprecated, use " +
					"distributor_received_decompressed_bytes_total instead.",
				Buckets: prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{"type", "tenant"},
		),
		receivedSamples: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_samples",
				Help:      "The number of samples per profile name received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(100, 100000, 30),
			},
			[]string{"type", "tenant"},
		),
		receivedSamplesBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_samples_bytes",
				Help:      "The size of samples without symbols received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"type", "tenant"},
		),
		receivedSymbolsBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_symbols_bytes",
				Help:      "The size of symbols received by the distributor.",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"type", "tenant"},
		),
		receivedDecompressedBytesTotal: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_decompressed_bytes_total",
				Help: "The total number of decompressed bytes per profile received by the distributor at different " +
					"processing stages. Valid stages are: " + allStages,
				Buckets: prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{
				"tenant",
				"stage",
			},
		),
	}
	if reg != nil {
		reg.MustRegister(
			m.receivedCompressedBytes,
			m.receivedDecompressedBytes,
			m.receivedSamples,
			m.receivedSamplesBytes,
			m.receivedSymbolsBytes,
			m.replicationFactor,
			m.receivedDecompressedBytesTotal,
		)
	}
	return m
}

func (m *metrics) observeProfileSize(tenant string, stage ReceiveStage, sz int64) {
	m.receivedDecompressedBytesTotal.WithLabelValues(tenant, string(stage)).Observe(float64(sz))
}
