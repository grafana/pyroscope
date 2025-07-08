package segmentwriter

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type segmentMetrics struct {
	segmentIngestBytes          *prometheus.HistogramVec
	segmentSizeBytes            *prometheus.HistogramVec
	headSizeBytes               *prometheus.HistogramVec
	segmentFlushWaitDuration    *prometheus.HistogramVec
	segmentFlushTimeouts        *prometheus.CounterVec
	storeMetadataDuration       *prometheus.HistogramVec
	storeMetadataDLQ            *prometheus.CounterVec
	segmentUploadDuration       *prometheus.HistogramVec
	segmentHedgedUploadDuration *prometheus.HistogramVec
	flushSegmentDuration        *prometheus.HistogramVec
	flushHeadsDuration          *prometheus.HistogramVec
	flushServiceHeadDuration    *prometheus.HistogramVec
	flushServiceHeadError       *prometheus.CounterVec
}

var (
	networkTimingBuckets    = prometheus.ExponentialBucketsRange(0.005, 4, 20)
	dataTimingBuckets       = prometheus.ExponentialBucketsRange(0.001, 1, 20)
	segmentFlushWaitBuckets = []float64{.1, .2, .3, .4, .5, .6, .7, .8, .9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2}
)

func newSegmentMetrics(reg prometheus.Registerer) *segmentMetrics {
	// TODO(kolesnikovae):
	//  - Use native histograms for all metrics
	//  - Remove unnecessary labels (e.g. shard)
	//  - Remove/merge/replace metrics
	//  - Rename to pyroscope_segment_writer_*
	//  - Add Help.
	m := &segmentMetrics{
		segmentIngestBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Subsystem: "segment_writer",
				Name:      "segment_ingest_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 20),
			},
			[]string{"shard", "tenant"}),
		segmentSizeBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Subsystem: "segment_writer",
				Name:      "segment_size_bytes",
				Buckets:   prometheus.ExponentialBucketsRange(100*1024, 100*1024*1024, 20),
			},
			[]string{"shard"}),

		segmentUploadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       "pyroscope",
			Subsystem:                       "segment_writer",
			Name:                            "upload_duration_seconds",
			Help:                            "Duration of segment upload requests.",
			Buckets:                         prometheus.ExponentialBucketsRange(0.001, 10, 30),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  32,
			NativeHistogramMinResetDuration: time.Minute * 15,
		}, []string{"status"}),

		segmentHedgedUploadDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       "pyroscope",
			Subsystem:                       "segment_writer",
			Name:                            "hedged_upload_duration_seconds",
			Help:                            "Duration of hedged segment upload requests.",
			Buckets:                         prometheus.ExponentialBucketsRange(0.001, 10, 30),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  32,
			NativeHistogramMinResetDuration: time.Minute * 15,
		}, []string{"status"}),

		storeMetadataDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       "pyroscope",
			Subsystem:                       "segment_writer",
			Name:                            "store_metadata_duration_seconds",
			Help:                            "Duration of store metadata requests.",
			Buckets:                         prometheus.ExponentialBucketsRange(0.001, 10, 30),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  32,
			NativeHistogramMinResetDuration: time.Minute * 15,
		}, []string{"status"}),

		storeMetadataDLQ: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "segment_writer",
			Name:      "store_metadata_dlq",
			Help:      "Number of store metadata entries that were sent to the DLQ.",
		}, []string{"status"}),

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
		reg.MustRegister(m.segmentSizeBytes)
		reg.MustRegister(m.storeMetadataDuration)
		reg.MustRegister(m.segmentFlushWaitDuration)
		reg.MustRegister(m.segmentFlushTimeouts)
		reg.MustRegister(m.storeMetadataDLQ)
		reg.MustRegister(m.segmentUploadDuration)
		reg.MustRegister(m.segmentHedgedUploadDuration)
		reg.MustRegister(m.flushHeadsDuration)
		reg.MustRegister(m.flushServiceHeadDuration)
		reg.MustRegister(m.flushServiceHeadError)
		reg.MustRegister(m.flushSegmentDuration)
		reg.MustRegister(m.headSizeBytes)
	}
	return m
}

func statusLabelValue(err error) string {
	if err == nil {
		return "success"
	}
	return "error"
}
