package compactor

import (
	"strconv"
	"sync/atomic"

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

type globalQueueStats struct {
	blocks         atomic.Int32
	queues         atomic.Int32
	blocksPerLevel []atomic.Int32 // Indexed by level for backlog calculation
}

func (g *globalQueueStats) AddBlock(level uint32, delta int32) {
	g.blocks.Add(1)
	g.blocksPerLevel[level].Add(delta)
}

type globalQueueStatsCollector struct {
	compactionQueue *compactionQueue

	blocks      *prometheus.Desc
	queues      *prometheus.Desc
	backlogJobs *prometheus.Desc
}

const globalQueueMetricsPrefix = "compaction_global_queue_"

func newGlobalQueueStatsCollector(compactionQueue *compactionQueue) *globalQueueStatsCollector {
	return &globalQueueStatsCollector{
		compactionQueue: compactionQueue,

		blocks: prometheus.NewDesc(
			globalQueueMetricsPrefix+"blocks_current",
			"The current total number of blocks across all queues.",
			nil, nil,
		),

		queues: prometheus.NewDesc(
			globalQueueMetricsPrefix+"queues_current",
			"The current total number of queues.",
			nil, nil,
		),

		backlogJobs: prometheus.NewDesc(
			globalQueueMetricsPrefix+"backlog_jobs_current",
			"The current estimated number of compaction jobs that are yet to be created.",
			nil, nil,
		),
	}
}

func (c *globalQueueStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.blocks
	ch <- c.queues
	ch <- c.backlogJobs
}

func (c *globalQueueStatsCollector) Collect(ch chan<- prometheus.Metric) {
	blocks := c.compactionQueue.globalStats.blocks.Load()
	queues := c.compactionQueue.globalStats.queues.Load()

	var totalBacklogJobs int
	for levelIdx, levelConfig := range c.compactionQueue.config.Levels {
		blocksAtLevel := c.compactionQueue.globalStats.blocksPerLevel[levelIdx].Load()
		maxBlocks := levelConfig.MaxBlocks
		if maxBlocks == 0 {
			// This is likely a misconfiguration, we'll just skip it.
			continue
		}
		totalBacklogJobs += int(blocksAtLevel) / int(maxBlocks)
	}

	ch <- prometheus.MustNewConstMetric(c.blocks, prometheus.GaugeValue, float64(blocks))
	ch <- prometheus.MustNewConstMetric(c.queues, prometheus.GaugeValue, float64(queues))
	ch <- prometheus.MustNewConstMetric(c.backlogJobs, prometheus.GaugeValue, float64(totalBacklogJobs))
}
