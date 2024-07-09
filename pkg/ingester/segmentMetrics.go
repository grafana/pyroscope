package ingester

import (
	"github.com/grafana/dskit/instrument"
	"github.com/prometheus/client_golang/prometheus"
)

type segmentMetrics struct {
	segmentIngestBytes       *prometheus.HistogramVec
	segmentBlockSizeBytes    *prometheus.HistogramVec
	storeMetaDuration        *prometheus.HistogramVec
	segmentFlushWaitDuration *prometheus.HistogramVec
	segmentFlushTimeouts     *prometheus.CounterVec
	storeMetaErrors          *prometheus.CounterVec
	blockUploadDuration      *prometheus.HistogramVec
}

func newSegmentMetrics(reg prometheus.Registerer) *segmentMetrics {
	m := &segmentMetrics{
		segmentIngestBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "segment_ingest_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"shard", "tenant", "service"}),
		segmentBlockSizeBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "segment_block_size_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(100*1024, 100*1024*1024, 30),
			},
			[]string{"shard"}),
		storeMetaDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_store_meta_duration_seconds",
			Buckets:   instrument.DefBuckets,
		}, []string{"shard"}),
		blockUploadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_block_upload_duration_seconds",
			Buckets:   instrument.DefBuckets,
		}, []string{"shard"}),

		storeMetaErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_store_meta_errors",
			}, []string{"shard"}),

		segmentFlushWaitDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_ingester_wait_duration_seconds",
			Buckets:   instrument.DefBuckets,
		}, []string{"tenant"}),
		segmentFlushTimeouts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_ingester_wait_timeouts",
			}, []string{"tenant"}),
	}

	if reg != nil {
		reg.MustRegister(m.segmentIngestBytes)
		reg.MustRegister(m.segmentBlockSizeBytes)
		reg.MustRegister(m.storeMetaDuration)
		reg.MustRegister(m.segmentFlushWaitDuration)
		reg.MustRegister(m.segmentFlushTimeouts)
		reg.MustRegister(m.storeMetaErrors)
		reg.MustRegister(m.blockUploadDuration)
	}
	return m
}
