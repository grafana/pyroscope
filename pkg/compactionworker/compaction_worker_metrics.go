package compactionworker

import "github.com/prometheus/client_golang/prometheus"

type compactionWorkerMetrics struct {
	jobsCompleted  *prometheus.CounterVec
	jobsInProgress *prometheus.GaugeVec
	jobDuration    *prometheus.HistogramVec
}

func newMetrics(r prometheus.Registerer) *compactionWorkerMetrics {
	m := &compactionWorkerMetrics{}

	m.jobsCompleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pyroscope_compaction_jobs_completed_count",
		Help: "Total number of compactions that were executed.",
	}, []string{"tenant", "shard", "level", "outcome"})
	m.jobsInProgress = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pyroscope_compaction_jobs_current",
		Help: "The number of active compaction jobs per level",
	}, []string{"tenant", "shard", "level"})
	m.jobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pyroscope_compaction_jobs_duration_seconds",
		Help:    "Duration of compaction job runs",
		Buckets: prometheus.ExponentialBuckets(1, 2, 14),
	}, []string{"tenant", "shard", "level", "outcome"})

	if r != nil {
		r.MustRegister(
			m.jobsCompleted,
			m.jobsInProgress,
			m.jobDuration,
		)
	}
	return m
}
