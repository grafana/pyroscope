package compactor

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction/compactor/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

var (
	_ compaction.Compactor = (*Compactor)(nil)
	_ compaction.Planner   = (*Compactor)(nil)
)

type Tombstones interface {
	GetExpiredTombstones(*bbolt.Tx, *raft.Log) iter.Iterator[*metastorev1.Tombstones]
}

type BlockQueueStore interface {
	StoreEntry(*bbolt.Tx, store.BlockEntry) error
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error
	ListEntries(*bbolt.Tx) iter.Iterator[store.BlockEntry]
}

type Compactor struct {
	strategy   Strategy
	queue      *compactionQueue
	store      BlockQueueStore
	tombstones Tombstones
}

type Config struct {
	Strategy Strategy
}

func NewCompactor(strategy Strategy, store BlockQueueStore, tombstones Tombstones) *Compactor {
	return &Compactor{
		strategy:   strategy,
		queue:      newCompactionQueue(strategy),
		store:      store,
		tombstones: tombstones,
	}
}

func (c *Compactor) AddBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
	e := store.BlockEntry{
		Index:      cmd.Index,
		AppendedAt: cmd.AppendedAt.UnixNano(),
		ID:         md.Id,
		Shard:      md.Shard,
		Level:      md.CompactionLevel,
		Tenant:     md.TenantId,
	}
	if err := c.store.StoreEntry(tx, e); err != nil {
		return err
	}
	c.enqueue(e)
	return nil
}

func (c *Compactor) enqueue(e store.BlockEntry) bool {
	return c.queue.push(e)
}

func (c *Compactor) NewPlan(tx *bbolt.Tx, cmd *raft.Log) compaction.Plan {
	return &plan{
		compactor:  c,
		tombstones: c.tombstones.GetExpiredTombstones(tx, cmd),
		blocks:     newBlockIter(),
	}
}

func (c *Compactor) UpdatePlan(tx *bbolt.Tx, _ *raft.Log, plan *raft_log.CompactionPlanUpdate) error {
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

func (c *Compactor) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	c.queue = newCompactionQueue(c.strategy)
	entries := c.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		c.enqueue(entries.At())
	}
	return entries.Err()
}
