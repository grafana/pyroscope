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
	StoreJob(*bbolt.Tx, *metastorev1.CompactionJob) error
	GetJob(tx *bbolt.Tx, name string) (*metastorev1.CompactionJob, error)
	GetCompactedBlocks(tx *bbolt.Tx, name string) (*raft_log.CompactedBlocks, error)
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
	return &compactionSchedule{
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

func (sc *Scheduler) AddJobs(tx *bbolt.Tx, planner compaction.Planner, jobs ...*metastorev1.CompactionJob) error {
	for _, job := range jobs {
		if err := sc.store.StoreJob(tx, job); err != nil {
			return err
		}
		if err := planner.Planned(tx, job); err != nil {
			return err
		}
	}
	return nil
}

func (sc *Scheduler) UpdateSchedule(tx *bbolt.Tx, planner compaction.Planner, jobs ...*raft_log.CompactionJobState) error {
	for _, job := range jobs {
		if job.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS {
			return sc.deleteJob(tx, planner, job)
		}
		if err := sc.store.UpdateJobState(tx, job); err != nil {
			return err
		}
		sc.queue.put(job)
	}
	// TODO: Bump all the stats here, right after the schedule update.
	return nil
}

func (sc *Scheduler) deleteJob(tx *bbolt.Tx, planner compaction.Planner, job *raft_log.CompactionJobState) error {
	if err := sc.store.DeleteJob(tx, job.Name); err != nil {
		return err
	}
	if err := sc.store.DeleteJobState(tx, job.Name); err != nil {
		return err
	}
	sc.queue.delete(job)
	return planner.Compacted(tx, job.CompactedBlocks)
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
