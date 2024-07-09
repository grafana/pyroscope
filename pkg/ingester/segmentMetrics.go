package ingester

import "github.com/prometheus/client_golang/prometheus"

type segmentMetrics struct {
	segmentIngestBytes *prometheus.HistogramVec
}

func newSegmentMetrics(reg prometheus.Registerer) *segmentMetrics {
	m := &segmentMetrics{
		segmentIngestBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "segment_ingest_bytes",
				Help:      "",
				Buckets:   prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30),
			},
			[]string{"shard", "tenant", "service"}),
	}
	if reg != nil {
		reg.MustRegister(m.segmentIngestBytes)
	}
	return m
}
