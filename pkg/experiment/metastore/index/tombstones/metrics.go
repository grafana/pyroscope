package tombstones

import (
	"github.com/prometheus/client_golang/prometheus"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

type metrics struct {
	tombstones *prometheus.GaugeVec
}

func newMetrics(r prometheus.Registerer) *metrics {
	m := &metrics{
		tombstones: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "index_tombstones",
			Help: "The number of tombstones in the queue.",
		}, []string{"tenant", "type"}),
	}

	util.Register(r,
		m.tombstones,
	)

	return m
}

const (
	tombstoneTypeBlocks = "blocks"
	tombstoneTypeShard  = "shard"
)

func (m *metrics) incrementTombstones(t *metastorev1.Tombstones) {
	if t.Blocks != nil {
		m.tombstones.WithLabelValues(t.Blocks.Tenant, tombstoneTypeBlocks).Inc()
	}
	if t.Shard != nil {
		m.tombstones.WithLabelValues(t.Shard.Tenant, tombstoneTypeShard).Inc()
	}
}

func (m *metrics) decrementTombstones(t *metastorev1.Tombstones) {
	if t.Blocks != nil {
		m.tombstones.WithLabelValues(t.Blocks.Tenant, tombstoneTypeBlocks).Dec()
	}
	if t.Shard != nil {
		m.tombstones.WithLabelValues(t.Shard.Tenant, tombstoneTypeShard).Dec()
	}
}
