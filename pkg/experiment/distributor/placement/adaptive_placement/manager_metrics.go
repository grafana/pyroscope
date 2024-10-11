package adaptive_placement

import (
	"github.com/prometheus/client_golang/prometheus"
)

type managerMetrics struct {
	rulesTotal prometheus.Gauge
	statsTotal prometheus.Gauge
	lastUpdate prometheus.Gauge

	datasetShardLimit          *prometheus.GaugeVec
	datasetShardUsage          *prometheus.GaugeVec
	datasetShardUsageBreakdown *prometheus.GaugeVec
}

func newManagerMetrics(reg prometheus.Registerer) *managerMetrics {
	m := &managerMetrics{
		lastUpdate: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_placement_rules_last_update_time",
			Help: "Second timestamp of the last successful update.",
		}),
		rulesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_placement_rules",
			Help: "Total number of rule entries.",
		}),
		statsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_placement_stats",
			Help: "Total number of stats entries.",
		}),

		datasetShardLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_placement_dataset_shard_limit",
			Help: "Maximum number of shards allowed for a dataset.",
		}, []string{"tenant", "dataset", "load_balancing"}),

		datasetShardUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_placement_dataset_shard_usage_bytes_per_second",
			Help: "Usage of the dataset in bytes per second.",
		}, []string{"tenant", "dataset"}),

		datasetShardUsageBreakdown: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_placement_dataset_shard_usage_per_shard_bytes_per_second",
			Help: "Usage of the dataset shard in bytes per second.",
		}, []string{"tenant", "dataset", "shard_id", "shard_owner"}),
	}
	if reg != nil {
		reg.MustRegister(
			m.lastUpdate,
			m.rulesTotal,
			m.statsTotal,
			m.datasetShardLimit,
			m.datasetShardUsage,
			m.datasetShardUsageBreakdown,
		)
	}
	return m
}
