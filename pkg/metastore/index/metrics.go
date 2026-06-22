package index

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	cacheRequests *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		cacheRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "metastore",
				Name:      "index_cache_requests_total",
				Help: "Total number of metastore index cache lookups, partitioned by cache and result. " +
					"The block cache has two tiers; result distinguishes which tier served the " +
					"lookup (read_hit / write_hit) or whether both tiers missed.",
			},
			[]string{"cache", "result"},
		),
	}
	if reg != nil {
		reg.MustRegister(m.cacheRequests)
	}
	return m
}

func (m *metrics) recordShardReadHit() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("shard_read", "hit").Inc()
}

func (m *metrics) recordShardReadMiss() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("shard_read", "miss").Inc()
}

func (m *metrics) recordShardWriteHit() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("shard_write", "hit").Inc()
}

func (m *metrics) recordShardWriteMiss() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("shard_write", "miss").Inc()
}

func (m *metrics) recordBlockReadHit() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("block", "read_hit").Inc()
}

func (m *metrics) recordBlockWriteHit() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("block", "write_hit").Inc()
}

func (m *metrics) recordBlockMiss() {
	if m == nil {
		return
	}
	m.cacheRequests.WithLabelValues("block", "miss").Inc()
}
