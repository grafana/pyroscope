package compactor

import (
	"flag"
	"time"

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
	ListTombstones(before time.Time) iter.Iterator[*metastorev1.Tombstones]
}

type BlockQueueStore interface {
	StoreEntry(*bbolt.Tx, store.BlockEntry) error
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error
	ListEntries(*bbolt.Tx) iter.Iterator[store.BlockEntry]
	CreateBuckets(*bbolt.Tx) error
}

type Config struct {
	Strategy
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	c.Strategy = DefaultStrategy()
	// TODO
}

type Compactor struct {
	config     Config
	queue      *compactionQueue
	store      BlockQueueStore
	tombstones Tombstones
}

func NewCompactor(config Config, store BlockQueueStore, tombstones Tombstones) *Compactor {
	return &Compactor{
		config:     config,
		queue:      newCompactionQueue(config.Strategy),
		store:      store,
		tombstones: tombstones,
	}
}

func NewStore() *store.BlockQueueStore {
	return store.NewBlockQueueStore()
}

func (c *Compactor) Compact(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
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

func (c *Compactor) NewPlan(_ *bbolt.Tx, cmd *raft.Log) compaction.Plan {
	before := cmd.AppendedAt.Add(-c.config.CleanupDelay)
	tombstones := c.tombstones.ListTombstones(before)
	return &plan{
		compactor:  c,
		tombstones: tombstones,
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

func (c *Compactor) Init(tx *bbolt.Tx) error {
	return c.store.CreateBuckets(tx)
}

func (c *Compactor) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	c.queue = newCompactionQueue(c.config.Strategy)
	entries := c.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		c.enqueue(entries.At())
	}
	return entries.Err()
}
