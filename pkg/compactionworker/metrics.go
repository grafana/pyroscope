package compactionworker

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/v2/pkg/util"
)

type workerMetrics struct {
	budgetWait         *prometheus.HistogramVec
	estimatedMemory    *prometheus.HistogramVec
	oversizedJobs      *prometheus.CounterVec
	jobsCompacting     *prometheus.GaugeVec
	compactionDuration *prometheus.HistogramVec
	jobsInProgress     *prometheus.GaugeVec
	jobsCompleted      *prometheus.CounterVec
	jobDuration        *prometheus.HistogramVec
	timeToCompaction   *prometheus.HistogramVec
	blocksDeleted      *prometheus.CounterVec
}

func durationOpts(name, help string) prometheus.HistogramOpts {
	return prometheus.HistogramOpts{Name: name, Help: help,
		Buckets:                     prometheus.ExponentialBucketsRange(0.001, 3600, 20),
		NativeHistogramBucketFactor: 1.1, NativeHistogramMaxBucketNumber: 50, NativeHistogramMinResetDuration: time.Hour}
}

// budgetCollector takes a single consistent snapshot per scrape. Reservations
// are estimates, not measured heap usage, and may exceed the limit for a lone oversized job.
type budgetCollector struct {
	budget *byteBudget
	descs  [5]*prometheus.Desc
}

func newBudgetCollector(b *byteBudget) *budgetCollector {
	c := &budgetCollector{budget: b}
	names := []string{"memory_budget_enabled", "memory_budget_limit_bytes", "memory_budget_reserved_bytes", "memory_budget_waiting_bytes", "memory_budget_waiting_jobs"}
	help := []string{"Whether memory budget enforcement is enabled.", "Resolved memory budget limit; zero when disabled.", "Estimated bytes reserved by admitted jobs, not actual memory usage.", "Estimated bytes of jobs waiting for budget admission.", "Jobs queued for budget admission, excluding the worker input queue."}
	for i := range names {
		c.descs[i] = prometheus.NewDesc(names[i], help[i], nil, nil)
	}
	return c
}

func (c *budgetCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
}

func (c *budgetCollector) Collect(ch chan<- prometheus.Metric) {
	var values [5]float64
	b := c.budget
	b.mu.Lock()
	if b.limit > 0 {
		values = [5]float64{1, float64(b.limit), float64(b.inUse), float64(b.waiting), float64(b.waiters.Len())}
	}
	b.mu.Unlock()
	for i, d := range c.descs {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, values[i])
	}
}

func newMetrics(r prometheus.Registerer) *workerMetrics {
	m := &workerMetrics{
		budgetWait: prometheus.NewHistogramVec(durationOpts("memory_budget_wait_duration_seconds", "Time spent queued for memory budget admission, excluding immediate acquisitions."), []string{"level", "outcome"}),
		estimatedMemory: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "job_estimated_memory_bytes", Help: "Estimated memory per job attempt, including when budget enforcement is disabled.",
			Buckets:                     prometheus.ExponentialBuckets(1024*1024, 4, 9),
			NativeHistogramBucketFactor: 1.1, NativeHistogramMaxBucketNumber: 50, NativeHistogramMinResetDuration: time.Hour,
		}, []string{"level"}),
		oversizedJobs:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "memory_budget_oversized_jobs_total", Help: "Admitted jobs whose estimated memory exceeds the enabled budget limit."}, []string{"level"}),
		jobsCompacting:     prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "jobs_compacting", Help: "Jobs currently executing compaction, excluding preparation and budget waits."}, []string{"level"}),
		compactionDuration: prometheus.NewHistogramVec(durationOpts("compaction_duration_seconds", "Time executing compaction, excluding preparation, budget waits and cleanup."), []string{"level", "status"}),
		jobsInProgress: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "jobs_in_progress",
			Help: "Jobs in progress, including metadata loading and memory budget waits.",
		}, []string{"tenant", "level"}),

		jobsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "jobs_completed_total",
			Help: "Total number of compaction jobs completed.",
		}, []string{"tenant", "level", "status"}),

		jobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "job_duration_seconds",
			Help: "Duration of job attempts, including metadata loading, memory budget waits and cleanup.",

			Buckets:                         prometheus.ExponentialBucketsRange(1, 300, 16),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"tenant", "level", "status"}),

		timeToCompaction: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "time_to_compaction_seconds",
			Help: "The time elapsed since the oldest compacted block was created.",

			Buckets:                         prometheus.ExponentialBucketsRange(1, 14400, 16),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"tenant", "level"}),

		blocksDeleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "blocks_deleted_total",
			Help: "Total number of block deletion attempts.",
		}, []string{"status"}),
	}

	util.Register(r,
		m.budgetWait, m.estimatedMemory, m.oversizedJobs, m.jobsCompacting, m.compactionDuration,
		m.jobsInProgress,
		m.jobsCompleted,
		m.jobDuration,
		m.timeToCompaction,
		m.blocksDeleted,
	)

	return m
}
