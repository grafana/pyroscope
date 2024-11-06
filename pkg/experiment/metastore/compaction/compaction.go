package compaction

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

// TODO(kolesnikovae): Consider delegating tombstone management to the planner.

type Planner interface {
	NewPlan(*bbolt.Tx) Plan

	AddBlocks(*bbolt.Tx, *raft.Log, ...*metastorev1.BlockMeta) error

	// Planned and Compacted methods are called by Scheduler
	// to communicate the progress back to the planner.
	Planned(*bbolt.Tx, *metastorev1.CompactionJob) error
	Compacted(*bbolt.Tx, *raft_log.CompactedBlocks) error
}

type Plan interface {
	CreateJob() (*metastorev1.CompactionJob, error)
}

type Scheduler interface {
	// NewSchedule is called to plan a schedule update. The proposed schedule
	// will then be submitted for Raft consensus, with the leader's schedule
	// being accepted as the final decision.
	// Implementation note: Schedule planning should be considered a read
	// operation and must have no side effects
	NewSchedule(*bbolt.Tx, *raft.Log) Schedule

	// AddJobs adds new jobs to the schedule. The jobs have no status yet:
	// corresponding entries should be added separately via UpdateSchedule.
	// The jobs were accepted by the raft quorum: the scheduler MUST add them.
	AddJobs(*bbolt.Tx, Planner, ...*metastorev1.CompactionJob) error
	// UpdateSchedule updates the state of existing jobs.
	// The change was accepted by the raft quorum: the scheduler MUST apply it.
	UpdateSchedule(*bbolt.Tx, Planner, ...*raft_log.CompactionJobState) error
}

type Schedule interface {
	// UpdateJob is called on behalf of the worker to update the job status.
	// A nil state should be interpreted as "no new lease": stop the work.
	// The scheduler must validate that the worker is allowed to update the job,
	// by comparing the fencing token of the job. Refer to the documentation for
	// details.
	UpdateJob(*metastorev1.CompactionJobStatusUpdate) (*raft_log.CompactionJobState, error)
	// AssignJob is called on behalf of the worker to request a new job.
	AssignJob() (*metastorev1.CompactionJob, *raft_log.CompactionJobState, error)
}
