package compaction

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/block/metadata"
)

type Compactor interface {
	// Compact enqueues a new block for compaction
	Compact(*bbolt.Tx, BlockEntry) error
}

type Planner interface {
	// NewPlan is used to plan new jobs. The proposed changes will then be
	// submitted for Raft consensus, with the leader's jobs being accepted
	// as the final decision.
	// Implementation: Plan must not change the state of the Planner.
	NewPlan(*raft.Log) Plan
	// UpdatePlan communicates the status of the compaction job to the planner.
	// Implementation: This method must be idempotent.
	UpdatePlan(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error
}

type Plan interface {
	// CreateJob creates a plan for a new job.
	CreateJob() (*raft_log.CompactionJobPlan, error)
}

type Scheduler interface {
	// NewSchedule is used to plan a schedule update. The proposed schedule
	// will then be submitted for Raft consensus, with the leader's schedule
	// being accepted as the final decision.
	// Implementation: Schedule must not change the state of the Scheduler.
	NewSchedule(*bbolt.Tx, *raft.Log) Schedule
	// UpdateSchedule adds new jobs and updates the state of existing ones.
	// Implementation: This method must be idempotent.
	UpdateSchedule(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error
}

// Schedule prepares changes to the compaction plan based on status updates
// from compaction workers. The standard sequence assumes that job updates
// (including lease renewals and completion reports) occur first, followed by
// the assignment of new jobs to workers. Only after these updates are new
// compaction jobs planned.
type Schedule interface {
	// UpdateJob is called on behalf of the worker to update the job status.
	// A nil response should be interpreted as "no new lease": stop the work.
	// The scheduler must validate that the worker is allowed to update the
	// job by comparing the fencing token of the job.
	// Refer to the documentation for details.
	UpdateJob(*raft_log.CompactionJobStatusUpdate) *raft_log.CompactionJobState
	// AssignJob is called on behalf of the worker to request a new job.
	AssignJob() (*raft_log.AssignedCompactionJob, error)
	// EvictJob is called on behalf of the planner to evict jobs that cannot
	// be assigned to workers, and free up resources for new jobs.
	EvictJob() *raft_log.CompactionJobState
	// AddJob is called on behalf of the planner to add a new job to the schedule.
	// The scheduler may decline the job by returning a nil state.
	AddJob(*raft_log.CompactionJobPlan) *raft_log.CompactionJobState
}

// BlockEntry represents a block metadata entry compaction operates on.
type BlockEntry struct {
	Index      uint64
	AppendedAt int64
	ID         string
	Tenant     string
	Shard      uint32
	Level      uint32
}

func NewBlockEntry(cmd *raft.Log, md *metastorev1.BlockMeta) BlockEntry {
	return BlockEntry{
		Index:      cmd.Index,
		AppendedAt: cmd.AppendedAt.UnixNano(),
		ID:         md.Id,
		Tenant:     metadata.Tenant(md),
		Shard:      md.Shard,
		Level:      md.CompactionLevel,
	}
}
