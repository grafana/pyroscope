package compactor

import (
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type schedulerStatsCollector struct {
	scheduler *Scheduler

	jobsTotal *prometheus.Desc
	oldestJob *prometheus.Desc
	newestJob *prometheus.Desc
}

func newSchedulerStatsCollector(sc *Scheduler) prometheus.Collector {
	return &schedulerStatsCollector{
		scheduler: sc,

		jobsTotal: prometheus.NewDesc(
			"compaction_scheduled_jobs_total",
			"The number of active aggregates.",
			[]string{"level", "status"},
			nil,
		),

		oldestJob: prometheus.NewDesc(
			"compaction_scheduled_max_job_age_seconds",
			"The oldest job age in seconds.",
			[]string{"level", "status"},
			nil,
		),

		newestJob: prometheus.NewDesc(
			"compaction_scheduled_min_job_age_seconds",
			"The newest job age in seconds.",
			[]string{"level", "status"},
			nil,
		),
	}
}

func (c *schedulerStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *schedulerStatsCollector) Collect(ch chan<- prometheus.Metric) {
	type levelStatusStats struct {
		jobs   float64
		oldest int64
		newest int64
	}

	c.scheduler.mu.Lock()
	levels := make([][]levelStatusStats, len(c.scheduler.levels))
	for i := range c.scheduler.levels {
		levels[i] = make([]levelStatusStats, 0, 5)
		for _, job := range c.scheduler.levels[i] {
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
	c.scheduler.mu.Unlock()

	for level, statuses := range levels {
		levelLabel := strconv.Itoa(level)
		for status, stats := range statuses {
			if stats.jobs > 0 {
				oldest := time.Since(time.Unix(0, stats.oldest)).Seconds()
				newest := time.Since(time.Unix(0, stats.newest)).Seconds()
				labels := []string{levelLabel, metastorev1.CompactionJobStatus(status).String()}
				ch <- prometheus.MustNewConstMetric(c.jobsTotal, prometheus.GaugeValue, stats.jobs, labels...)
				ch <- prometheus.MustNewConstMetric(c.oldestJob, prometheus.GaugeValue, oldest, labels...)
				ch <- prometheus.MustNewConstMetric(c.newestJob, prometheus.GaugeValue, newest, labels...)
			}
		}
	}
}
