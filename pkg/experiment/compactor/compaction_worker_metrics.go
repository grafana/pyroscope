package compactor

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type compactionWorkerMetrics struct {
	jobsInProgress   *prometheus.GaugeVec
	jobsCompleted    *prometheus.CounterVec
	jobDuration      *prometheus.HistogramVec
	timeToCompaction *prometheus.HistogramVec
}

func newMetrics(r prometheus.Registerer) *compactionWorkerMetrics {
	m := &compactionWorkerMetrics{
		jobsInProgress: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "jobs_in_progress",
			Help: "The number of active compaction jobs currently running.",
		}, []string{"tenant", "level"}),

		jobsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "jobs_completed_total",
			Help: "Total number of compaction jobs completed.",
		}, []string{"tenant", "level", "status"}),

		jobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "job_duration_seconds",
			Help: "Duration of compaction job runs",

			Buckets:                         prometheus.ExponentialBuckets(1, 300, 16),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  16,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"tenant", "level", "status"}),

		timeToCompaction: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "time_to_compaction_seconds",
			Help: "The time elapsed since the oldest compacted block was created.",

			Buckets:                         prometheus.ExponentialBuckets(1, 3600, 16),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  16,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"tenant", "level"}),
	}

	util.Register(r,
		m.jobsInProgress,
		m.jobsCompleted,
		m.jobDuration,
		m.timeToCompaction,
	)

	return m
}
