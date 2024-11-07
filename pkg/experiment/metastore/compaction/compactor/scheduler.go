package compactor

import (
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
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
	StoreJob(*bbolt.Tx, *raft_log.CompactionJobPlan) error
	GetJob(tx *bbolt.Tx, name string) (*raft_log.CompactionJobPlan, error)
	DeleteJob(tx *bbolt.Tx, name string) error
	// Jobs are not loaded in memory.

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
	// Although the scheduler is supposed to be used by a single planner
	// in a synchronous manner, we still need to protect it from concurrent
	// read accesses, such as stats collection, and listing jobs for debug
	// purposes. This is a write-intensive path, so we use a regular mutex.
	mu    sync.Mutex
	queue *jobQueue
	store JobStore
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

func (sc *Scheduler) NewSchedule(tx *bbolt.Tx, raft *raft.Log) compaction.Schedule {
	return &schedule{
		tx:        tx,
		raft:      raft,
		scheduler: sc,
		assigner: &jobAssigner{
			raft:   raft,
			config: sc.config,
			queue:  sc.queue,
		},
	}
}

func (sc *Scheduler) UpdateSchedule(tx *bbolt.Tx, p compaction.Planner, update *raft_log.CompactionPlanUpdate) error {
	for _, job := range update.NewJobs {
		if err := sc.store.StoreJob(tx, job); err != nil {
			return err
		}
	}
	for _, state := range update.ScheduleUpdates {
		if state.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS {
			return sc.evict(tx, p, state)
		}
		if err := sc.store.UpdateJobState(tx, state); err != nil {
			return err
		}
		sc.queue.put(state)
	}

	// TODO: Bump all the stats here, right after the schedule update.
	return p.Scheduled(tx, update.NewJobs...)
}

func (sc *Scheduler) evict(tx *bbolt.Tx, p compaction.Planner, state *raft_log.CompactionJobState) error {
	job, err := sc.store.GetJob(tx, state.Name)
	if err != nil {
		return err
	}
	if err = sc.store.DeleteJob(tx, state.Name); err != nil {
		return err
	}
	if err = sc.store.DeleteJobState(tx, state.Name); err != nil {
		return err
	}
	sc.queue.delete(state)
	return p.Compacted(tx, job)
}

func (sc *Scheduler) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	sc.queue.reset()
	entries := sc.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		sc.queue.put(entries.At())
	}
	return entries.Err()
}
