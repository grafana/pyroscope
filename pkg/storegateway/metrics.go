package storegateway

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Synced *prometheus.GaugeVec

	blockLoads        prometheus.Counter
	blockLoadFailures prometheus.Counter
	blockDrops        prometheus.Counter
	blockDropFailures prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	var m Metrics
	m.Synced = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "blocks_meta",
			Name:      "synced",
			Help:      "Number of block metadata synced",
		},
		[]string{"state"},
	)
	m.blockLoads = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_bucket_store_block_loads_total",
		Help: "Total number of remote block loading attempts.",
	})
	m.blockLoadFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_bucket_store_block_load_failures_total",
		Help: "Total number of failed remote block loading attempts.",
	})
	m.blockDrops = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_bucket_store_block_drops_total",
		Help: "Total number of local blocks that were dropped.",
	})
	m.blockDropFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_bucket_store_block_drop_failures_total",
		Help: "Total number of local blocks that failed to be dropped.",
	})
	reg.MustRegister(m.Synced, m.blockDropFailures, m.blockDrops, m.blockLoadFailures, m.blockLoads)
	return &m
}
