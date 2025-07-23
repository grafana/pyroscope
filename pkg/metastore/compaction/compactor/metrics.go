package compactor

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type queueStatsCollector struct {
	stats *queueStats

	blocks   *prometheus.Desc
	batches  *prometheus.Desc
	rejected *prometheus.Desc
	missed   *prometheus.Desc
}

const blockQueueMetricsPrefix = "compaction_block_queue_"

func newQueueStatsCollector(staged *stagedBlocks) *queueStatsCollector {
	constLabels := map[string]string{
		"tenant": staged.key.tenant,
		"shard":  strconv.FormatUint(uint64(staged.key.shard), 10),
		"level":  strconv.FormatUint(uint64(staged.key.level), 10),
	}

	return &queueStatsCollector{
		stats: staged.stats,

		blocks: prometheus.NewDesc(
			blockQueueMetricsPrefix+"blocks",
			"The total number of blocks in the queue.",
			nil, constLabels,
		),

		batches: prometheus.NewDesc(
			blockQueueMetricsPrefix+"batches",
			"The total number of block batches in the queue.",
			nil, constLabels,
		),

		rejected: prometheus.NewDesc(
			blockQueueMetricsPrefix+"push_rejected_total",
			"The total number of blocks rejected on push.",
			nil, constLabels,
		),

		missed: prometheus.NewDesc(
			blockQueueMetricsPrefix+"delete_missed_total",
			"The total number of blocks missed on delete.",
			nil, constLabels,
		),
	}
}

func (b *queueStatsCollector) Describe(c chan<- *prometheus.Desc) {
	c <- b.blocks
	c <- b.batches
	c <- b.rejected
	c <- b.missed
}

func (b *queueStatsCollector) Collect(m chan<- prometheus.Metric) {
	m <- prometheus.MustNewConstMetric(b.blocks, prometheus.GaugeValue, float64(b.stats.blocks.Load()))
	m <- prometheus.MustNewConstMetric(b.batches, prometheus.GaugeValue, float64(b.stats.batches.Load()))
	m <- prometheus.MustNewConstMetric(b.rejected, prometheus.CounterValue, float64(b.stats.rejected.Load()))
	m <- prometheus.MustNewConstMetric(b.missed, prometheus.CounterValue, float64(b.stats.missed.Load()))
}
