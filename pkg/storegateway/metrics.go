package storegateway

import (
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/util"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	registerer prometheus.Registerer

	blockMetrics *phlaredb.BlocksMetrics

	synced            *prometheus.GaugeVec
	blockLoads        prometheus.Counter
	blockLoadFailures prometheus.Counter
	blockDrops        prometheus.Counter
	blockDropFailures prometheus.Counter
}

func NewBucketStoreMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		registerer:   reg,
		blockMetrics: phlaredb.NewBlocksMetrics(reg),

		synced: util.RegisterOrGet(reg, prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "blocks_meta",
				Name:      "synced",
				Help:      "Number of block metadata synced",
			}, []string{"state"})),

		blockLoads: util.RegisterOrGet(reg, prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_bucket_store_block_loads_total",
			Help: "Total number of remote block loading attempts.",
		})),

		blockLoadFailures: util.RegisterOrGet(reg, prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_bucket_store_block_load_failures_total",
			Help: "Total number of failed remote block loading attempts.",
		})),

		blockDrops: util.RegisterOrGet(reg, prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_bucket_store_block_drops_total",
			Help: "Total number of local blocks that were dropped.",
		})),

		blockDropFailures: util.RegisterOrGet(reg, prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_bucket_store_block_drop_failures_total",
			Help: "Total number of local blocks that failed to be dropped.",
		})),
	}
}

func (m *Metrics) Unregister() {
	m.blockMetrics.Unregister()
	for _, c := range []prometheus.Collector{
		m.synced,
		m.blockLoads,
		m.blockLoadFailures,
		m.blockDrops,
		m.blockDropFailures,
	} {
		m.registerer.Unregister(c)
	}
}
