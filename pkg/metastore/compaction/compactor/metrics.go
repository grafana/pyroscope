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

type globalQueueStatsCollector struct {
	compactionQueue *compactionQueue

	blocks      *prometheus.Desc
	queues      *prometheus.Desc
	backlogJobs *prometheus.Desc
}

const globalQueueMetricsPrefix = "compaction_global_queue_"

func newBlockQueueStatsCollector(compactionQueue *compactionQueue) *globalQueueStatsCollector {
	variableLabels := []string{"level"}

	return &globalQueueStatsCollector{
		compactionQueue: compactionQueue,

		blocks: prometheus.NewDesc(
			globalQueueMetricsPrefix+"blocks_current",
			"The current total number of blocks across all queues.",
			variableLabels, nil,
		),

		queues: prometheus.NewDesc(
			globalQueueMetricsPrefix+"queues_current",
			"The current total number of queues.",
			variableLabels, nil,
		),

		backlogJobs: prometheus.NewDesc(
			globalQueueMetricsPrefix+"backlog_jobs_current",
			"The current estimated number of compaction jobs that are yet to be created.",
			variableLabels, nil,
		),
	}
}

func (c *globalQueueStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.blocks
	ch <- c.queues
	ch <- c.backlogJobs
}

func (c *globalQueueStatsCollector) Collect(ch chan<- prometheus.Metric) {
	numLevels := len(c.compactionQueue.config.Levels)
	for levelIdx := 0; levelIdx < numLevels; levelIdx++ {
		var blocksForLevel int32
		var queuesForLevel int
		var backlogJobsForLevel int

		levelLabel := strconv.Itoa(levelIdx)

		if levelIdx < len(c.compactionQueue.levels) && c.compactionQueue.levels[levelIdx] != nil {
			queue := c.compactionQueue.levels[levelIdx]

			maxBlocks := queue.config.maxBlocks(uint32(levelIdx))
			if maxBlocks == 0 {
				// This is likely a misconfiguration, we'll just skip it.
				continue
			}

			for _, staged := range queue.staged {
				blocks := staged.stats.blocks.Load()
				blocksForLevel += blocks
				queuesForLevel++

				backlogJobs := int(blocks) / int(maxBlocks)
				backlogJobsForLevel += backlogJobs
			}
		}

		ch <- prometheus.MustNewConstMetric(c.blocks, prometheus.GaugeValue, float64(blocksForLevel), levelLabel)
		ch <- prometheus.MustNewConstMetric(c.queues, prometheus.GaugeValue, float64(queuesForLevel), levelLabel)
		ch <- prometheus.MustNewConstMetric(c.backlogJobs, prometheus.GaugeValue, float64(backlogJobsForLevel), levelLabel)
	}
}
