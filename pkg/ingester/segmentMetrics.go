package ingester

import (
	"github.com/grafana/dskit/instrument"
	"github.com/prometheus/client_golang/prometheus"
)

type segmentMetrics struct {
	segmentIngestBytes         *prometheus.HistogramVec
	segmentBlockSizeBytes      *prometheus.HistogramVec
	storeMetaDuration          *prometheus.HistogramVec
	segmentFlushWaitDuration   *prometheus.HistogramVec
	segmentFlushTimeouts       *prometheus.CounterVec
	storeMetaErrors            *prometheus.CounterVec
	blockUploadDuration        *prometheus.HistogramVec
	flushSegmentsDuration      prometheus.Histogram
	flushSegmentDuration       *prometheus.HistogramVec
	flushHeadsDuration         *prometheus.HistogramVec
	flushServiceHeadDuration   *prometheus.HistogramVec
	flushServiceHeadError      *prometheus.CounterVec
	flushServiceHeadEmptyCount *prometheus.CounterVec
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
			Buckets:   prometheus.ExponentialBucketsRange(0.001, 2, 30),
		}, []string{"shard"}),
		blockUploadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_block_upload_duration_seconds",
			Buckets:   prometheus.ExponentialBucketsRange(0.001, 5, 30),
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
		flushHeadsDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_heads_duration_seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 1.22, 20),
		}, []string{"shard"}),
		flushServiceHeadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_service_head_duration_seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 1.22, 20),
		}, []string{"shard", "tenant", "service"}),
		flushSegmentDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_segment_duration_seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 1.22, 20),
		}, []string{"shard"}),
		flushSegmentsDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_segments_duration_seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 1.22, 20),
		}),

		flushServiceHeadError: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_flush_service_head_errors",
			}, []string{"shard", "tenant", "service"}),
		flushServiceHeadEmptyCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_flush_service_head_empty_count",
			}, []string{"shard", "tenant", "service"}),
	}

	if reg != nil {
		reg.MustRegister(m.segmentIngestBytes)
		reg.MustRegister(m.segmentBlockSizeBytes)
		reg.MustRegister(m.storeMetaDuration)
		reg.MustRegister(m.segmentFlushWaitDuration)
		reg.MustRegister(m.segmentFlushTimeouts)
		reg.MustRegister(m.storeMetaErrors)
		reg.MustRegister(m.blockUploadDuration)
		reg.MustRegister(m.flushHeadsDuration)
		reg.MustRegister(m.flushServiceHeadDuration)
		reg.MustRegister(m.flushServiceHeadError)
		reg.MustRegister(m.flushServiceHeadEmptyCount)
		reg.MustRegister(m.flushSegmentDuration)
		reg.MustRegister(m.flushSegmentsDuration)
	}
	return m
}
