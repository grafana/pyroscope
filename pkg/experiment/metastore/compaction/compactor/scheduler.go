package compactor

import (
	"time"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/iter"
)

var _ compaction.Scheduler = (*Scheduler)(nil)

// Compaction job scheduler. Jobs are prioritized by the compaction level, and
// the deadline time.
//
// Compaction workers own jobs while they are in progress. Ownership handling is
// implemented using lease deadlines and fencing tokens:
// https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html

// JobStore does not really store jobs as they are: it explicitly
// distinguishes between the job and the job state.
//
// Implementation note: block metadata should never be stored in StoreJob:
// those are already stored in the metadata index.
type JobStore interface {
	StoreJobPlan(*bbolt.Tx, *raft_log.CompactionJobPlan) error
	GetJobPlan(tx *bbolt.Tx, name string) (*raft_log.CompactionJobPlan, error)
	DeleteJobPlan(tx *bbolt.Tx, name string) error

	GetJobState(tx *bbolt.Tx, name string) (*raft_log.CompactionJobState, error)
	UpdateJobState(*bbolt.Tx, *raft_log.CompactionJobState) error
	DeleteJobState(tx *bbolt.Tx, name string) error
	ListEntries(*bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState]
}

type SchedulerConfig struct {
	MaxFailures   uint32
	LeaseDuration time.Duration
}

type Scheduler struct {
	config SchedulerConfig
	queue  *jobQueue
	store  JobStore
}

// NewScheduler creates a scheduler with the given lease duration.
// Typically, callers should update jobs at the interval not exceeding
// the half of the lease duration.
func NewScheduler(config SchedulerConfig, store JobStore) *Scheduler {
	return &Scheduler{
		config: config,
		store:  store,
		queue:  newJobQueue(),
	}
}

func (sc *Scheduler) NewSchedule(tx *bbolt.Tx, cmd *raft.Log) compaction.Schedule {
	return &schedule{
		tx:        tx,
		token:     cmd.Index,
		now:       cmd.AppendedAt,
		scheduler: sc,
		updates:   make(map[string]*raft_log.CompactionJobUpdate),
	}
}

func (sc *Scheduler) UpdateSchedule(tx *bbolt.Tx, _ *raft.Log, update *raft_log.CompactionPlanUpdate) error {
	for _, job := range update.NewJobs {
		if err := sc.store.StoreJobPlan(tx, job.Plan); err != nil {
			return err
		}
		if err := sc.store.UpdateJobState(tx, job.State); err != nil {
			return err
		}
		sc.queue.put(job.State)
	}

	for _, job := range update.AssignedJobs {
		if err := sc.store.UpdateJobState(tx, job.State); err != nil {
			return err
		}
		sc.queue.put(job.State)
	}

	for _, job := range update.CompletedJobs {
		name := job.Plan.Name
		if err := sc.store.DeleteJobPlan(tx, name); err != nil {
			return err
		}
		if err := sc.store.DeleteJobState(tx, name); err != nil {
			return err
		}
		sc.queue.delete(name)
	}

	return nil
}

func (sc *Scheduler) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	sc.queue = newJobQueue()
	entries := sc.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		sc.queue.put(entries.At())
	}
	return entries.Err()
}
