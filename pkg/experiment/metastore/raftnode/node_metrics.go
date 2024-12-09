package raftnode

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type metrics struct {
	apply prometheus.Histogram
	read  prometheus.Histogram
	state *prometheus.GaugeVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		apply: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "raft_apply_command_duration_seconds",
			Help:                            "Duration of applying a command to the Raft log",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramZeroThreshold:    0,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
			NativeHistogramMaxZeroThreshold: 0,
		}),

		read: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "raft_read_index_wait_duration_seconds",
			Help:                            "Duration of the Raft log read index wait",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramZeroThreshold:    0,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
			NativeHistogramMaxZeroThreshold: 0,
		}),

		state: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "raft_state",
				Help: "Current Raft state",
			},
			[]string{"state"},
		),
	}

	if reg != nil {
		util.RegisterOrGet(reg, m.apply)
		util.RegisterOrGet(reg, m.read)
		util.RegisterOrGet(reg, m.state)
	}

	return m
}
