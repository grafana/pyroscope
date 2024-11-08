package compaction

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type Compactor interface {
	AddBlock(*bbolt.Tx, *raft.Log, *metastorev1.BlockMeta) error
}

type Planner interface {
	NewPlan(*bbolt.Tx) Plan
	// Scheduled must be called for each job after it is scheduled
	// to remove the job from future plans.
	Scheduled(*bbolt.Tx, ...*raft_log.CompactionJobPlan) error
}

type Plan interface {
	CreateJob() (*raft_log.CompactionJobPlan, error)
}

type Scheduler interface {
	// NewSchedule is called to plan a schedule update. The proposed schedule
	// will then be submitted for Raft consensus, with the leader's schedule
	// being accepted as the final decision.
	// Implementation note: Schedule planning should be considered a read
	// operation and must have no side effects.
	NewSchedule(*bbolt.Tx, *raft.Log) Schedule

	// UpdateSchedule adds new jobs and updates state of existing ones.
	// The change was accepted by the raft quorum: the scheduler MUST apply it.
	UpdateSchedule(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error
}

type Schedule interface {
	// UpdateJob is called on behalf of the worker to update the job status.
	// A nil state should be interpreted as "no new lease": stop the work.
	// The scheduler must validate that the worker is allowed to update the job,
	// by comparing the fencing token of the job. Refer to the documentation for
	// details.
	UpdateJob(*metastorev1.CompactionJobStatusUpdate) (*Job, error)

	// AssignJob is called on behalf of the worker to request a new job.
	// This method should be called before any UpdateJob to avoid assigning
	// the same job unassigned as a result of the update (e.g, a job failure).
	AssignJob() (*Job, error)

	// AddJob is called on behalf of the planner to add a new job to the schedule.
	AddJob(*raft_log.CompactionJobPlan) (*Job, error)
}

// Job represents an update of the compaction plan and the job schedule.
// Job plan and state may be nil, depending on the context:
//   - If the job is created, both state and plan are present.
//   - If the job is completed, the state is nil, and the complete plan is present.
//   - If the job is in progress, the state is present, and the plan is nil.
type Job struct {
	State *raft_log.CompactionJobState
	Plan  *raft_log.CompactionJobPlan
}
