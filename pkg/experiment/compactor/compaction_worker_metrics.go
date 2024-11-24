package compactor

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type metrics struct {
	jobsCompleted  *prometheus.CounterVec
	jobsInProgress *prometheus.GaugeVec
	jobDuration    *prometheus.HistogramVec
}

func newMetrics(r prometheus.Registerer) *metrics {
	m := &metrics{
		jobsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "jobs_completed_total",
			Help: "Total number of compaction jobs completed.",
		}, []string{"tenant", "shard", "level", "status"}),

		jobsInProgress: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "jobs_in_progress",
			Help: "The number of active compaction jobs currently running.",
		}, []string{"tenant", "shard", "level"}),

		jobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "job_duration_seconds",
			Help:    "Duration of compaction job runs",
			Buckets: prometheus.ExponentialBuckets(1, 2, 14),
		}, []string{"tenant", "shard", "level", "status"}),
	}

	util.Register(r,
		m.jobsCompleted,
		m.jobsInProgress,
		m.jobDuration,
	)

	return m
}
