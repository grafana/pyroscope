package scheduler

import (
	"flag"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction/scheduler/store"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/util"
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

	StoreJobState(*bbolt.Tx, *raft_log.CompactionJobState) error
	DeleteJobState(tx *bbolt.Tx, name string) error
	ListEntries(*bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState]

	CreateBuckets(*bbolt.Tx) error
}

type Config struct {
	MaxFailures   uint64        `yaml:"compaction_max_failures" doc:""`
	LeaseDuration time.Duration `yaml:"compaction_job_lease_duration" doc:""`
	MaxQueueSize  uint64        `yaml:"compaction_max_job_queue_size" doc:""`
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.Uint64Var(&c.MaxFailures, prefix+"compaction-max-failures", 3, "")
	f.DurationVar(&c.LeaseDuration, prefix+"compaction-job-lease-duration", 15*time.Second, "")
	f.Uint64Var(&c.MaxQueueSize, prefix+"compaction-max-job-queue-size", 10000, "")
}

type Scheduler struct {
	config Config
	store  JobStore
	// Although the job queue is only accessed for writes
	// synchronously, the mutex is needed to collect stats.
	mu    sync.Mutex
	queue *schedulerQueue
}

// NewScheduler creates a scheduler with the given lease duration.
// Typically, callers should update jobs at the interval not exceeding
// the half of the lease duration.
func NewScheduler(config Config, store JobStore, reg prometheus.Registerer) *Scheduler {
	s := &Scheduler{
		config: config,
		store:  store,
		queue:  newJobQueue(),
	}
	collector := newStatsCollector(s)
	util.RegisterOrGet(reg, collector)
	return s
}

func NewStore() *store.JobStore {
	return store.NewJobStore()
}

func (sc *Scheduler) NewSchedule(tx *bbolt.Tx, cmd *raft.Log) compaction.Schedule {
	return &schedule{
		tx:        tx,
		token:     cmd.Index,
		now:       cmd.AppendedAt,
		scheduler: sc,
		updates:   make(map[string]*raft_log.CompactionJobState),
	}
}

func (sc *Scheduler) UpdateSchedule(tx *bbolt.Tx, update *raft_log.CompactionPlanUpdate) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for _, job := range update.EvictedJobs {
		name := job.State.Name
		if err := sc.store.DeleteJobPlan(tx, name); err != nil {
			return err
		}
		if err := sc.store.DeleteJobState(tx, name); err != nil {
			return err
		}
		sc.queue.evict(name)
	}

	for _, job := range update.NewJobs {
		if err := sc.store.StoreJobPlan(tx, job.Plan); err != nil {
			return err
		}
		if err := sc.store.StoreJobState(tx, job.State); err != nil {
			return err
		}
		sc.queue.put(job.State)
	}

	for _, job := range update.UpdatedJobs {
		if err := sc.store.StoreJobState(tx, job.State); err != nil {
			return err
		}
		sc.queue.put(job.State)
	}

	for _, job := range update.AssignedJobs {
		if err := sc.store.StoreJobState(tx, job.State); err != nil {
			return err
		}
		sc.queue.put(job.State)
	}

	for _, job := range update.CompletedJobs {
		name := job.State.Name
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

func (sc *Scheduler) Init(tx *bbolt.Tx) error {
	return sc.store.CreateBuckets(tx)
}

func (sc *Scheduler) Restore(tx *bbolt.Tx) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	// Reset in-memory state before loading entries from the store.
	sc.queue.reset()
	entries := sc.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		sc.queue.put(entries.At())
	}
	// Zero all stats updated during Restore.
	sc.queue.resetStats()
	return entries.Err()
}
