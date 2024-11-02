package metastore

import (
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

type jobQueueStatsCollector struct {
	queue *jobQueue

	jobsTotal *prometheus.Desc
	oldestJob *prometheus.Desc
	newestJob *prometheus.Desc
}

func newJobQueueStatsCollector(queue *jobQueue) prometheus.Collector {
	return &jobQueueStatsCollector{
		queue: queue,

		jobsTotal: prometheus.NewDesc(
			"pyroscope_compaction_queue_jobs_total",
			"The number of active aggregates.",
			[]string{"level", "status"},
			nil,
		),

		oldestJob: prometheus.NewDesc(
			"pyroscope_compaction_queue_max_job_age_seconds",
			"The oldest job age in seconds.",
			[]string{"level", "status"},
			nil,
		),

		newestJob: prometheus.NewDesc(
			"pyroscope_compaction_queue_min_job_age_seconds",
			"The newest job age in seconds.",
			[]string{"level", "status"},
			nil,
		),
	}
}

func (c *jobQueueStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *jobQueueStatsCollector) Collect(ch chan<- prometheus.Metric) {
	type levelStatusStats struct {
		jobs   float64
		oldest int64
		newest int64
	}

	c.queue.mu.Lock()
	levels := make([][]levelStatusStats, len(c.queue.levels))
	for i := range c.queue.levels {
		levels[i] = make([]levelStatusStats, 0, 5)
		for _, job := range c.queue.levels[i] {
			j := int(job.Status)
			levels[i] = slices.Grow(levels[i], j+1)[:j+1]
			levels[i][j].jobs++
			levels[i][j].newest = max(levels[i][j].newest, job.AddedAt)
			levels[i][j].oldest = min(levels[i][j].oldest, job.AddedAt)
			if levels[i][j].oldest == 0 {
				levels[i][j].oldest = job.AddedAt
			}
		}
	}
	c.queue.mu.Unlock()

	for level, statuses := range levels {
		levelLabel := strconv.Itoa(level)
		for status, stats := range statuses {
			if stats.jobs > 0 {
				oldest := time.Since(time.Unix(0, stats.oldest)).Seconds()
				newest := time.Since(time.Unix(0, stats.newest)).Seconds()
				labels := []string{levelLabel, compactionpb.CompactionStatus(status).String()}
				ch <- prometheus.MustNewConstMetric(c.jobsTotal, prometheus.GaugeValue, stats.jobs, labels...)
				ch <- prometheus.MustNewConstMetric(c.oldestJob, prometheus.GaugeValue, oldest, labels...)
				ch <- prometheus.MustNewConstMetric(c.newestJob, prometheus.GaugeValue, newest, labels...)
			}
		}
	}
}
