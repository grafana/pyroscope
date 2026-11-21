package raftnode

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/v2/pkg/util"
)

type metrics struct {
	apply               prometheus.Histogram
	read                prometheus.Histogram
	state               *prometheus.GaugeVec
	logStoreWrite       prometheus.Histogram
	logStoreWriteRecent prometheus.Summary
	logStoreTimeout     prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		apply: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "raft_apply_command_duration_seconds",
			Help:                            "Duration of applying a command to the Raft log",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}),

		read: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "raft_read_index_wait_duration_seconds",
			Help:                            "Duration of the Raft log read index wait",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}),

		state: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "raft_state",
				Help: "Current Raft state",
			},
			[]string{"state"},
		),

		logStoreWrite: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "raft_log_store_write_duration_seconds",
			Help:                            "Duration of log store write operations",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}),

		logStoreTimeout: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "raft_log_store_write_timeouts_total",
			Help: "Number of log store write operations that timed out",
		}),

		// Local election signal, kept separate from the exported WAL histogram.
		logStoreWriteRecent: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       "raft_log_store_write_recent_duration_seconds",
			Help:       "Recent WAL write latency used to delay ordinary elections",
			Objectives: map[float64]float64{0.99: 0.001},
			MaxAge:     5 * time.Minute,
		}),
	}

	if reg != nil {
		util.RegisterOrGet(reg, m.apply)
		util.RegisterOrGet(reg, m.read)
		util.RegisterOrGet(reg, m.state)
		util.RegisterOrGet(reg, m.logStoreWrite)
		util.RegisterOrGet(reg, m.logStoreTimeout)
	}

	return m
}

func (m *metrics) observeLogStoreWrite(seconds float64) {
	m.logStoreWrite.Observe(seconds)
	m.logStoreWriteRecent.Observe(seconds)
}
