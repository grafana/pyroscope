package distributor

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type ReceiveMetricStage int

const (
	ReceiveMetricStageReceived ReceiveMetricStage = iota
	ReceiveMetricStageSampled
	ReceiveMetricStageNormalized
	totalStages
)

func (s ReceiveMetricStage) String() string {
	switch s {
	case ReceiveMetricStageSampled:
		return "sampled"
	case ReceiveMetricStageReceived:
		return "received"
	case ReceiveMetricStageNormalized:
		return "normalized"
	default:
		panic(fmt.Sprintf("unexpected ReceiveMetricStage value: %d", s))
	}
}

func ReceiveMetricStageFromString(s string) ReceiveMetricStage {
	switch s {
	case "sampled":
		return ReceiveMetricStageSampled
	case "received":
		return ReceiveMetricStageReceived
	case "normalized":
		return ReceiveMetricStageNormalized
	default:
		return ReceiveMetricStageSampled
	}
}

type ReceivedMetricsMeter struct {
	metric      *prometheus.HistogramVec
	tenant      string
	tenantStage ReceiveMetricStage
	sizes       [totalStages]int64
	set         [totalStages]bool
}

func (m *ReceivedMetricsMeter) Record(stage ReceiveMetricStage, size int64) {
	m.set[stage] = true
	m.sizes[stage] = size
}
func (m *ReceivedMetricsMeter) Observe() {
	if !m.set[ReceiveMetricStageReceived] {
		panic("ReceiveMetricStageReceived metric stage not set. It should always be set")
	}
	m.observe(ReceiveMetricStageReceived)
	if !m.set[ReceiveMetricStageSampled] {
		return
	}
	m.observe(ReceiveMetricStageSampled)
	if !m.set[ReceiveMetricStageNormalized] {
		m.sizes[ReceiveMetricStageNormalized] = m.sizes[ReceiveMetricStageSampled]
		m.set[ReceiveMetricStageNormalized] = true
	}
	m.observe(ReceiveMetricStageNormalized)

}

func (m *ReceivedMetricsMeter) observe(stage ReceiveMetricStage) {
	sz := m.sizes[stage]
	if !m.set[stage] {
		panic(fmt.Sprintf("Received metric stage %d not set", stage))
	}
	m.metric.WithLabelValues(m.tenant, stage.String(), fmt.Sprint(stage == m.tenantStage)).Observe(float64(sz))
}

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
				Help:      "The number of decompressed bytes per profiles received by the distributor after limits/sampling checks.",
				Buckets:   prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
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
				Help:      "The total number of decompressed bytes per profile received by the distributor at different processing stages.",
				Buckets:   prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{"tenant", "stage", "tenant_stage"},
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
