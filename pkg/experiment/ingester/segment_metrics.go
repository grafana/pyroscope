package ingester

import (
	"github.com/prometheus/client_golang/prometheus"
)

type segmentMetrics struct {
	segmentIngestBytes       *prometheus.HistogramVec
	segmentBlockSizeBytes    *prometheus.HistogramVec
	headSizeBytes            *prometheus.HistogramVec
	storeMetaDuration        *prometheus.HistogramVec
	segmentFlushWaitDuration *prometheus.HistogramVec
	segmentFlushTimeouts     *prometheus.CounterVec
	storeMetaErrors          *prometheus.CounterVec
	storeMetaDLQ             *prometheus.CounterVec
	blockUploadDuration      *prometheus.HistogramVec
	flushSegmentDuration     *prometheus.HistogramVec
	flushHeadsDuration       *prometheus.HistogramVec
	flushServiceHeadDuration *prometheus.HistogramVec
	flushServiceHeadError    *prometheus.CounterVec
}

var (
	networkTimingBuckets    = prometheus.ExponentialBucketsRange(0.005, 4, 20)
	dataTimingBuckets       = prometheus.ExponentialBucketsRange(0.001, 1, 20)
	segmentFlushWaitBuckets = []float64{.1, .2, .3, .4, .5, .6, .7, .8, .9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2}
)

func newSegmentMetrics(reg prometheus.Registerer) *segmentMetrics {

	m := &segmentMetrics{
		segmentIngestBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "segment_ingest_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 20),
			},
			[]string{"shard", "tenant"}),
		segmentBlockSizeBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "segment_block_size_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(100*1024, 100*1024*1024, 20),
			},
			[]string{"shard"}),
		storeMetaDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_store_meta_duration_seconds",
			Buckets:   networkTimingBuckets,
		}, []string{"shard"}),
		blockUploadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_block_upload_duration_seconds",
			Buckets:   networkTimingBuckets,
		}, []string{"shard"}),

		storeMetaErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_store_meta_errors",
			}, []string{"shard"}),
		storeMetaDLQ: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_store_meta_dlq",
			}, []string{"shard", "status"}),

		segmentFlushWaitDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_ingester_wait_duration_seconds",
			Buckets:   segmentFlushWaitBuckets,
		}, []string{"tenant"}),
		segmentFlushTimeouts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_ingester_wait_timeouts",
			}, []string{"tenant"}),
		flushHeadsDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_heads_duration_seconds",
			Buckets:   dataTimingBuckets,
		}, []string{"shard"}),
		flushServiceHeadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_service_head_duration_seconds",
			Buckets:   dataTimingBuckets,
		}, []string{"shard", "tenant"}),
		flushSegmentDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "segment_flush_segment_duration_seconds",
			Buckets:   networkTimingBuckets,
		}, []string{"shard"}),

		flushServiceHeadError: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Name:      "segment_flush_service_head_errors",
			}, []string{"shard", "tenant"}),
		headSizeBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "segment_head_size_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 100*1024*1024, 30),
			}, []string{"shard", "tenant"}),
	}

	if reg != nil {
		reg.MustRegister(m.segmentIngestBytes)
		reg.MustRegister(m.segmentBlockSizeBytes)
		reg.MustRegister(m.storeMetaDuration)
		reg.MustRegister(m.segmentFlushWaitDuration)
		reg.MustRegister(m.segmentFlushTimeouts)
		reg.MustRegister(m.storeMetaErrors)
		reg.MustRegister(m.storeMetaDLQ)
		reg.MustRegister(m.blockUploadDuration)
		reg.MustRegister(m.flushHeadsDuration)
		reg.MustRegister(m.flushServiceHeadDuration)
		reg.MustRegister(m.flushServiceHeadError)
		reg.MustRegister(m.flushSegmentDuration)
		reg.MustRegister(m.headSizeBytes)
	}
	return m
}
