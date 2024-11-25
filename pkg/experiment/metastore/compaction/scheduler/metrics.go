package scheduler

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type statsCollector struct {
	s *Scheduler

	addedTotal      *prometheus.Desc
	completedTotal  *prometheus.Desc
	assignedTotal   *prometheus.Desc
	reassignedTotal *prometheus.Desc

	// Gauge showing the job queue status breakdown.
	jobs *prometheus.Desc
}

const schedulerQueueMetricsPrefix = "compaction_scheduler_queue_"

func newStatsCollector(s *Scheduler) *statsCollector {
	variableLabels := []string{"level"}
	statusGaugeLabels := append(variableLabels, "status")
	return &statsCollector{
		s: s,

		jobs: prometheus.NewDesc(
			schedulerQueueMetricsPrefix+"jobs",
			"The total number of jobs in the queue.",
			statusGaugeLabels, nil,
		),

		addedTotal: prometheus.NewDesc(
			schedulerQueueMetricsPrefix+"added_jobs_total",
			"The total number of jobs added to the queue.",
			variableLabels, nil,
		),
		completedTotal: prometheus.NewDesc(
			schedulerQueueMetricsPrefix+"completed_jobs_total",
			"The total number of jobs completed.",
			variableLabels, nil,
		),
		assignedTotal: prometheus.NewDesc(
			schedulerQueueMetricsPrefix+"assigned_jobs_total",
			"The total number of jobs assigned.",
			variableLabels, nil,
		),
		reassignedTotal: prometheus.NewDesc(
			schedulerQueueMetricsPrefix+"reassigned_jobs_total",
			"The total number of jobs reassigned.",
			variableLabels, nil,
		),
	}
}

func (c *statsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.jobs
	ch <- c.addedTotal
	ch <- c.completedTotal
	ch <- c.assignedTotal
	ch <- c.reassignedTotal
}

func (c *statsCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range c.collectMetrics() {
		ch <- m
	}
}

func (c *statsCollector) collectMetrics() []prometheus.Metric {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()

	metrics := make([]prometheus.Metric, 0, 8*len(c.s.queue.levels))
	for i, q := range c.s.queue.levels {
		var stats queueStats
		for _, e := range *q.jobs {
			switch {
			case e.Status == 0:
				stats.unassigned++
			case c.s.config.MaxFailures > 0 && uint64(e.Failures) >= c.s.config.MaxFailures:
				stats.failed++
			case e.Failures > 0:
				stats.reassigned++
			default:
				stats.assigned++
			}
		}

		// Update stored gauges. Those are not used at the moment,
		// but can help planning schedule updates in the future.
		q.stats.assigned = stats.assigned
		q.stats.unassigned = stats.unassigned
		q.stats.reassigned = stats.reassigned
		q.stats.failed = stats.failed

		// Counters are updated on access.
		stats.addedTotal = q.stats.addedTotal
		stats.completedTotal = q.stats.completedTotal
		stats.assignedTotal = q.stats.assignedTotal
		stats.reassignedTotal = q.stats.reassignedTotal

		level := strconv.Itoa(i)
		metrics = append(metrics,
			prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue, float64(stats.assigned), level, "assigned"),
			prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue, float64(stats.unassigned), level, "unassigned"),
			prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue, float64(stats.reassigned), level, "reassigned"),
			prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue, float64(stats.failed), level, "failed"),
			prometheus.MustNewConstMetric(c.addedTotal, prometheus.CounterValue, float64(stats.addedTotal), level),
			prometheus.MustNewConstMetric(c.completedTotal, prometheus.CounterValue, float64(stats.completedTotal), level),
			prometheus.MustNewConstMetric(c.assignedTotal, prometheus.CounterValue, float64(stats.assignedTotal), level),
			prometheus.MustNewConstMetric(c.reassignedTotal, prometheus.CounterValue, float64(stats.reassignedTotal), level),
		)
	}

	return metrics
}
