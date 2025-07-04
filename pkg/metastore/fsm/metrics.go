package fsm

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type metrics struct {
	boltDBPersistSnapshotDuration prometheus.Histogram
	boltDBPersistSnapshotSize     prometheus.Gauge
	boltDBRestoreSnapshotDuration prometheus.Histogram
	fsmRestoreSnapshotDuration    prometheus.Histogram
	fsmApplyCommandSize           *prometheus.HistogramVec
	fsmApplyCommandDuration       *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	var dataTimingBuckets = prometheus.ExponentialBucketsRange(0.01, 20, 48)
	m := &metrics{
		boltDBPersistSnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "boltdb_persist_snapshot_duration_seconds",
			Buckets:                         dataTimingBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}),

		boltDBPersistSnapshotSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "boltdb_persist_snapshot_size_bytes",
		}),

		boltDBRestoreSnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "boltdb_restore_snapshot_duration_seconds",
			Buckets:                         dataTimingBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}),

		fsmRestoreSnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "fsm_restore_snapshot_duration_seconds",
			Buckets:                         dataTimingBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}),

		fsmApplyCommandSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:                            "fsm_apply_command_size_bytes",
			Buckets:                         prometheus.ExponentialBucketsRange(8, 64<<10, 48),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"command"}),

		fsmApplyCommandDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:                            "fsm_apply_command_duration_seconds",
			Buckets:                         dataTimingBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"command"}),
	}
	if reg != nil {
		util.RegisterOrGet(reg, m.boltDBPersistSnapshotSize)
		util.RegisterOrGet(reg, m.boltDBPersistSnapshotDuration)
		util.RegisterOrGet(reg, m.boltDBRestoreSnapshotDuration)
		util.RegisterOrGet(reg, m.fsmRestoreSnapshotDuration)
		util.RegisterOrGet(reg, m.fsmApplyCommandSize)
		util.RegisterOrGet(reg, m.fsmApplyCommandDuration)
	}
	return m
}
