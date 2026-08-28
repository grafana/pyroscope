package distributor

import (
	"fmt"
	"time"

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
	receivedSamples                *prometheus.HistogramVec
	receivedSamplesBytes           *prometheus.HistogramVec
	receivedSymbolsBytes           *prometheus.HistogramVec
	replicationFactor              prometheus.Gauge
	receivedDecompressedBytesTotal *prometheus.HistogramVec
	profilesReceived               *prometheus.CounterVec
	parseDuration                  *prometheus.HistogramVec
	pushBatchSeries                *prometheus.HistogramVec
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
				Namespace:                       "pyroscope",
				Name:                            "distributor_received_compressed_bytes",
				Help:                            "The number of compressed bytes per profile received by the distributor.",
				Buckets:                         prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{"type", "tenant"},
		),
		receivedSamples: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:                       "pyroscope",
				Name:                            "distributor_received_samples",
				Help:                            "The number of samples per profile name received by the distributor.",
				Buckets:                         prometheus.ExponentialBucketsRange(100, 100000, 30),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{"type", "tenant"},
		),
		receivedSamplesBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:                       "pyroscope",
				Name:                            "distributor_received_samples_bytes",
				Help:                            "The size of samples without symbols received by the distributor.",
				Buckets:                         prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{"type", "tenant"},
		),
		receivedSymbolsBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:                       "pyroscope",
				Name:                            "distributor_received_symbols_bytes",
				Help:                            "The size of symbols received by the distributor.",
				Buckets:                         prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{"type", "tenant"},
		),
		receivedDecompressedBytesTotal: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "distributor_received_decompressed_bytes_total",
				Help: "The total number of decompressed bytes per profile received by the distributor at different " +
					"processing stages. Valid stages are: " + allStages,
				Buckets:                         prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{
				"tenant",
				"stage",
			},
		),
		profilesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "distributor_profiles_received_total",
				Help:      "The total number of profiles received by the distributor, broken down by OpenTelemetry instrumentation scope.",
			},
			[]string{"tenant", "scope_name", "scope_version"},
		),
		parseDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:                       "pyroscope",
				Name:                            "distributor_parse_duration_seconds",
				Help:                            "Duration of profile parsing (JFR or pprof) per ingest request in the distributor.",
				Buckets:                         prometheus.ExponentialBucketsRange(0.001, 10, 30),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{"type", "tenant"},
		),
		pushBatchSeries: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:                       "pyroscope",
				Name:                            "distributor_push_batch_series",
				Help:                            "Number of series per batched push request (PushBatch call).",
				Buckets:                         prometheus.ExponentialBuckets(1, 2, 13),
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  50,
				NativeHistogramMinResetDuration: time.Hour,
			},
			[]string{"tenant"},
		),
	}
	if reg != nil {
		reg.MustRegister(
			m.receivedCompressedBytes,
			m.receivedSamples,
			m.receivedSamplesBytes,
			m.receivedSymbolsBytes,
			m.replicationFactor,
			m.receivedDecompressedBytesTotal,
			m.profilesReceived,
			m.parseDuration,
			m.pushBatchSeries,
		)
	}
	return m
}

func (m *metrics) observeProfileSize(tenant string, stage ReceiveStage, sz int64) {
	m.receivedDecompressedBytesTotal.WithLabelValues(tenant, string(stage)).Observe(float64(sz))
}
