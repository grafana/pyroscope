package metastore

import (
	"github.com/grafana/dskit/instrument"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type metastoreMetrics struct {
	boltDBPersistSnapshotDuration  prometheus.Histogram
	boltDBRestoreSnapshotDuration  prometheus.Histogram
	fsmRestoreSnapshotDuration     prometheus.Histogram
	fsmApplyCommandHandlerDuration prometheus.Histogram
	raftAddBlockDuration           prometheus.Histogram
	raftAddRecoveredBlockDuration  prometheus.Histogram
}

func newMetastoreMetrics(reg prometheus.Registerer) *metastoreMetrics {
	var dataTimingBuckets = prometheus.ExponentialBucketsRange(0.01, 20, 48)
	m := &metastoreMetrics{
		boltDBPersistSnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "metastore_boltdb_persist_snapshot_duration_seconds",
			//Buckets:   dataTimingBuckets,
			Buckets: instrument.DefBuckets,
		}),
		boltDBRestoreSnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "metastore_boltdb_restore_snapshot_duration_seconds",
			Buckets:   dataTimingBuckets,
		}),
		fsmRestoreSnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "metastore_fsm_restore_snapshot_duration_seconds",
			Buckets:   dataTimingBuckets,
		}),
		fsmApplyCommandHandlerDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "metastore_fsm_apply_command_handler_duration_seconds",
			Buckets:   dataTimingBuckets,
		}),
		raftAddBlockDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "metastore_raft_add_block_duration_seconds",
			Buckets:   dataTimingBuckets,
		}),
		raftAddRecoveredBlockDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "metastore_raft_add_recovered_block_duration_seconds",
			Buckets:   dataTimingBuckets,
		}),
	}
	if reg != nil {
		util.RegisterOrGet(reg, m.boltDBPersistSnapshotDuration)
		util.RegisterOrGet(reg, m.boltDBRestoreSnapshotDuration)
		util.RegisterOrGet(reg, m.fsmRestoreSnapshotDuration)
		util.RegisterOrGet(reg, m.fsmApplyCommandHandlerDuration)
		util.RegisterOrGet(reg, m.raftAddBlockDuration)
		util.RegisterOrGet(reg, m.raftAddRecoveredBlockDuration)
	}
	return m
}
