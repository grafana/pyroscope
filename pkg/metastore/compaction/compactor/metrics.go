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

type levelQueueStats struct {
	blocks  atomic.Int32
	queues  atomic.Int32
	batches atomic.Int32
}

type globalQueueStats struct {
	// Immutable slice of stable counters, published atomically for scrapes.
	// Queue mutations (including growth) have a single writer: the Raft FSM.
	levels atomic.Pointer[[]*levelQueueStats]
}

func newGlobalQueueStats(numLevels int) *globalQueueStats {
	g := new(globalQueueStats)
	levels := make([]*levelQueueStats, numLevels)
	for i := range levels {
		levels[i] = new(levelQueueStats)
	}
	g.levels.Store(&levels)
	return g
}

func (g *globalQueueStats) ensureLevel(level uint32) {
	current := *g.levels.Load()
	if uint64(level) < uint64(len(current)) {
		return
	}
	levels := make([]*levelQueueStats, int(level)+1)
	copy(levels, current)
	for i := len(current); i < len(levels); i++ {
		levels[i] = new(levelQueueStats)
	}
	g.levels.Store(&levels)
}

func (g *globalQueueStats) AddBlocks(key compactionKey, delta int32) {
	(*g.levels.Load())[key.level].blocks.Add(delta)
}

func (g *globalQueueStats) AddQueues(key compactionKey, delta int32) {
	(*g.levels.Load())[key.level].queues.Add(delta)
}

func (g *globalQueueStats) AddBatches(key compactionKey, delta int32) {
	(*g.levels.Load())[key.level].batches.Add(delta)
}

func (g *globalQueueStats) reset() {
	for _, level := range *g.levels.Load() {
		level.blocks.Store(0)
		level.queues.Store(0)
		level.batches.Store(0)
	}
}

type globalQueueStatsCollector struct {
	compactionQueue *compactionQueue

	blocks  *prometheus.Desc
	queues  *prometheus.Desc
	batches *prometheus.Desc
}

const globalQueueMetricsPrefix = "compaction_global_queue_"

func newGlobalQueueStatsCollector(compactionQueue *compactionQueue) *globalQueueStatsCollector {
	variableLabels := []string{"level"}

	return &globalQueueStatsCollector{
		compactionQueue: compactionQueue,

		blocks: prometheus.NewDesc(
			globalQueueMetricsPrefix+"blocks_current",
			"The current number of blocks across all queues, for a compaction level.",
			variableLabels, nil,
		),

		queues: prometheus.NewDesc(
			globalQueueMetricsPrefix+"queues_current",
			"The current number of queues, for a compaction level.",
			variableLabels, nil,
		),

		batches: prometheus.NewDesc(
			globalQueueMetricsPrefix+"batches_current",
			"The current number of batches (jobs that are not yet created), for a compaction level.",
			variableLabels, nil,
		),
	}
}

func (c *globalQueueStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.blocks
	ch <- c.queues
	ch <- c.batches
}

func (c *globalQueueStatsCollector) Collect(ch chan<- prometheus.Metric) {
	for levelIdx, stats := range *c.compactionQueue.globalStats.levels.Load() {
		blocksAtLevel := stats.blocks.Load()
		queuesAtLevel := stats.queues.Load()
		batchesAtLevel := stats.batches.Load()

		levelLabel := strconv.Itoa(levelIdx)

		ch <- prometheus.MustNewConstMetric(c.blocks, prometheus.GaugeValue, float64(blocksAtLevel), levelLabel)
		ch <- prometheus.MustNewConstMetric(c.queues, prometheus.GaugeValue, float64(queuesAtLevel), levelLabel)
		ch <- prometheus.MustNewConstMetric(c.batches, prometheus.GaugeValue, float64(batchesAtLevel), levelLabel)
	}
}
