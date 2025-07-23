package compactor

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/metastore/compaction/compactor/store"
)

var (
	_ compaction.Compactor = (*Compactor)(nil)
	_ compaction.Planner   = (*Compactor)(nil)
)

type Tombstones interface {
	ListTombstones(before time.Time) iter.Iterator[*metastorev1.Tombstones]
}

type BlockQueueStore interface {
	StoreEntry(*bbolt.Tx, compaction.BlockEntry) error
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error
	ListEntries(*bbolt.Tx) iter.Iterator[compaction.BlockEntry]
	CreateBuckets(*bbolt.Tx) error
}

type Compactor struct {
	config     Config
	queue      *compactionQueue
	store      BlockQueueStore
	tombstones Tombstones
}

func NewCompactor(
	config Config,
	store BlockQueueStore,
	tombstones Tombstones,
	reg prometheus.Registerer,
) *Compactor {
	queue := newCompactionQueue(config, reg)
	return &Compactor{
		config:     config,
		queue:      queue,
		store:      store,
		tombstones: tombstones,
	}
}

func NewStore() *store.BlockQueueStore {
	return store.NewBlockQueueStore()
}

func (c *Compactor) Compact(tx *bbolt.Tx, entry compaction.BlockEntry) error {
	if int(entry.Level) >= len(c.config.Levels) {
		return nil
	}
	if err := c.store.StoreEntry(tx, entry); err != nil {
		return err
	}
	c.enqueue(entry)
	return nil
}

func (c *Compactor) enqueue(e compaction.BlockEntry) bool {
	return c.queue.push(e)
}

func (c *Compactor) NewPlan(cmd *raft.Log) compaction.Plan {
	now := cmd.AppendedAt.UnixNano()
	before := cmd.AppendedAt.Add(-c.config.CleanupDelay)
	tombstones := c.tombstones.ListTombstones(before)
	return &plan{
		compactor:  c,
		tombstones: tombstones,
		blocks:     newBlockIter(),
		now:        now,
	}
}

func (c *Compactor) UpdatePlan(tx *bbolt.Tx, plan *raft_log.CompactionPlanUpdate) error {
	for _, job := range plan.NewJobs {
		// Delete source blocks from the compaction queue.
		k := compactionKey{
			tenant: job.Plan.Tenant,
			shard:  job.Plan.Shard,
			level:  job.Plan.CompactionLevel,
		}
		staged := c.queue.blockQueue(k.level).stagedBlocks(k)
		for _, b := range job.Plan.SourceBlocks {
			e := staged.delete(b)
			if e == zeroBlockEntry {
				continue
			}
			if err := c.store.DeleteEntry(tx, e.index, e.id); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Compactor) Init(tx *bbolt.Tx) error {
	return c.store.CreateBuckets(tx)
}

func (c *Compactor) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	c.queue.reset()
	entries := c.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		c.enqueue(entries.At())
	}
	return entries.Err()
}
