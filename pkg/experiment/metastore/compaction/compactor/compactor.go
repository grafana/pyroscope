package compactor

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction/compactor/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

var (
	_ compaction.Compactor = (*Compactor)(nil)
	_ compaction.Planner   = (*Compactor)(nil)
)

type TombstoneStore interface {
	Exists(*metastorev1.BlockMeta) bool
	AddTombstones(*bbolt.Tx, *raft.Log, *metastorev1.Tombstones) error
	GetTombstones(*bbolt.Tx, *raft.Log) (*metastorev1.Tombstones, error)
	DeleteTombstones(*bbolt.Tx, *raft.Log, *metastorev1.Tombstones) error
	ListEntries(*bbolt.Tx) iter.Iterator[store.BlockEntry]
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
	tombstones TombstoneStore
}

type Config struct {
	Strategy Strategy
}

func NewCompactor(strategy Strategy, store BlockQueueStore, tombstones TombstoneStore) *Compactor {
	return &Compactor{
		strategy:   strategy,
		queue:      newCompactionQueue(strategy),
		store:      store,
		tombstones: tombstones,
	}
}

func (p *Compactor) AddBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
	if p.tombstones.Exists(md) {
		return compaction.ErrAlreadyCompacted
	}
	e := store.BlockEntry{
		Index:      cmd.Index,
		AppendedAt: cmd.AppendedAt.UnixNano(),
		ID:         md.Id,
		Shard:      md.Shard,
		Level:      md.CompactionLevel,
		Tenant:     md.TenantId,
	}
	if err := p.store.StoreEntry(tx, e); err != nil {
		return err
	}
	p.enqueue(e)
	return nil
}

func (p *Compactor) DeleteBlocks(tx *bbolt.Tx, cmd *raft.Log, blocks ...*metastorev1.BlockMeta) error {
	tombstones := &metastorev1.Tombstones{Blocks: make([]string, len(blocks))}
	for i := range blocks {
		tombstones.Blocks[i] = block.ObjectPath(blocks[i])
	}
	return p.tombstones.AddTombstones(tx, cmd, tombstones)
}

func (p *Compactor) enqueue(e store.BlockEntry) bool {
	return p.queue.push(e)
}

func (p *Compactor) NewPlan(tx *bbolt.Tx, cmd *raft.Log) compaction.Plan {
	return &plan{
		tx:        tx,
		cmd:       cmd,
		compactor: p,
		blocks:    newBlockIter(),
	}
}

func (p *Compactor) UpdatePlan(tx *bbolt.Tx, cmd *raft.Log, plan *raft_log.CompactionPlanUpdate) error {
	for _, job := range plan.NewJobs {
		// Delete source blocks from the compaction queue.
		k := compactionKey{
			tenant: job.Plan.Tenant,
			shard:  job.Plan.Shard,
			level:  job.Plan.CompactionLevel,
		}
		staged := p.queue.blockQueue(k.level).stagedBlocks(k)
		for _, b := range job.Plan.SourceBlocks {
			e := staged.delete(b)
			if e == zeroBlockEntry {
				continue
			}
			if err := p.store.DeleteEntry(tx, e.index, e.id); err != nil {
				return err
			}
		}
		// Delete tombstones that are scheduled for removal.
		if err := p.tombstones.DeleteTombstones(tx, cmd, job.Plan.Tombstones); err != nil {
			return err
		}
	}

	return nil
}

func (p *Compactor) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	p.queue = newCompactionQueue(p.strategy)
	entries := p.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		// The only error can be returned here is ErrAlreadyCompacted,
		// which is impossible, unless there is a major bug in the
		// block queue and/or in the store implementation.
		if !p.enqueue(entries.At()) {
			panic("compactor: block duplicate in the queue")
		}
	}
	return entries.Err()
}
