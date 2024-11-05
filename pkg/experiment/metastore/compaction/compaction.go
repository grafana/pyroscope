package compaction

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type Planner interface {
	AddBlocks(*bbolt.Tx, *raft.Log, ...*metastorev1.BlockMeta) error
	NewPlan(*bbolt.Tx) Plan
	// Planned and Compacted methods is the way Scheduler communicates to planner.
	Planned(*bbolt.Tx, *metastorev1.CompactionJob) error
	Compacted(*bbolt.Tx, *raft_log.CompactedBlocks) error
}

type Plan interface {
	CreateJob() *metastorev1.CompactionJob
}

type Scheduler interface {
	AddJobs(*bbolt.Tx, Planner, ...*metastorev1.CompactionJob) error
	UpdateSchedule(*bbolt.Tx, Planner, ...*raft_log.CompactionJobState) error
	NewSchedule(*bbolt.Tx, *raft.Log) Schedule
}

type Schedule interface {
	UpdateJob(*metastorev1.CompactionJobStatusUpdate) *raft_log.CompactionJobState
	AssignJob() (*metastorev1.CompactionJob, *raft_log.CompactionJobState)
}
