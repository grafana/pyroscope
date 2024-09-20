package segmentwriterclient

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	sentBytes *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		// Note that the number of shards per tenant is limited.
		// The same for the "addr" limit: a shard resides on a single address,
		// ideally; in practice, if the segment writer is not available, the
		// shard may be relocated.
		sentBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_segment_writer_client_sent_bytes",
			Buckets: prometheus.ExponentialBucketsRange(100, 100<<20, 30),
			Help:    "Number of bytes sent by the segment writer client.",
		}, []string{"shard", "tenant", "addr"}),
	}
	if reg != nil {
		reg.MustRegister(m.sentBytes)
	}
	return m
}
