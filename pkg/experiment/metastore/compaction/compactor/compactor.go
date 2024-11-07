package compactor

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/iter"
)

var _ compaction.Compactor = (*Compactor)(nil)
var _ compaction.Planner = (*Compactor)(nil)

type BlockQueueStore interface {
	StoreEntry(*bbolt.Tx, BlockEntry) error
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error
	ListEntries(*bbolt.Tx) iter.Iterator[BlockEntry]
}

type TombstoneStore interface {
	StoreTombstones(*bbolt.Tx, *metastorev1.BlockList) error
	DeleteTombstones(*bbolt.Tx, *metastorev1.BlockList) error
	ListEntries(tx *bbolt.Tx, tenant string, shard uint32) iter.Iterator[string]
}

type BlockEntry struct {
	Index  uint64
	ID     string
	Shard  uint32
	Level  uint32
	Tenant string
}

type Compactor struct {
	strategy   strategy
	queue      *compactionQueue
	store      BlockQueueStore
	tombstones TombstoneStore
}

func NewCompactor(store BlockQueueStore) *Compactor {
	config := defaultCompactionStrategy
	return &Compactor{
		strategy: config,
		store:    store,
		queue:    newCompactionQueue(config),
	}
}

func (p *Compactor) AddBlocks(tx *bbolt.Tx, cmd *raft.Log, blocks ...*metastorev1.BlockMeta) error {
	for _, md := range blocks {
		if err := p.addBlock(tx, cmd, md); err != nil {
			return err
		}
	}
	return nil
}

func (p *Compactor) addBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
	if !p.strategy.canCompact(md) {
		return nil
	}
	e := BlockEntry{
		Index:  cmd.Index,
		ID:     md.Id,
		Shard:  md.Shard,
		Level:  md.CompactionLevel,
		Tenant: md.TenantId,
	}
	if err := p.store.StoreEntry(tx, e); err != nil {
		return err
	}
	p.enqueue(e)
	return nil
}

func (p *Compactor) enqueue(e BlockEntry) {
	c := compactionKey{
		tenant: e.Tenant,
		shard:  e.Shard,
		level:  e.Level,
	}
	b := blockEntry{
		raftIndex: e.Index,
		id:        e.ID,
	}
	if p.queue.push(c, b) {
		// Another entry with the same block ID already exists.
		// TODO: Add a log message, bump a metric, etc.
	}
}

func (p *Compactor) NewPlan(tx *bbolt.Tx) compaction.Plan {
	return &plan{
		tx:        tx,
		compactor: p,
		blocks:    newBlockIter(),
	}
}

func (p *Compactor) Scheduled(tx *bbolt.Tx, jobs ...*raft_log.CompactionJobPlan) error {
	for _, job := range jobs {
		if err := p.planned(tx, job); err != nil {
			return err
		}
	}
	return nil
}

func (p *Compactor) planned(tx *bbolt.Tx, job *raft_log.CompactionJobPlan) error {
	k := compactionKey{
		tenant: job.Tenant,
		shard:  job.Shard,
		level:  job.CompactionLevel,
	}
	staged := p.queue.stagedBlocks(k)
	for _, block := range job.SourceBlocks {
		e := staged.delete(block)
		if e == zeroBlockEntry {
			continue
		}
		if err := p.store.DeleteEntry(tx, e.raftIndex, e.id); err != nil {
			return err
		}
	}
	return nil
}

func (p *Compactor) Compacted(tx *bbolt.Tx, jobs ...*raft_log.CompactionJobPlan) error {
	for _, job := range jobs {
		if err := p.compacted(tx, job); err != nil {
			return err
		}
	}
	return nil
}

func (p *Compactor) compacted(tx *bbolt.Tx, jobs ...*raft_log.CompactionJobPlan) error {
	for _, job := range jobs {
		tombstones := &metastorev1.BlockList{
			Tenant: job.Tenant,
			Shard:  job.Shard,
			Blocks: job.DeletedBlocks,
		}
		if err := p.tombstones.DeleteTombstones(tx, tombstones); err != nil {
			return err
		}
	}
	return nil
}

func (p *Compactor) DeleteBlocks(tx *bbolt.Tx, _ *raft.Log, blocks *metastorev1.BlockList) error {
	return p.tombstones.StoreTombstones(tx, blocks)
}

func (p *Compactor) Restore(tx *bbolt.Tx) error {
	// Reset in-memory state before loading entries from the store.
	p.queue = newCompactionQueue(p.strategy)
	entries := p.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		p.enqueue(entries.At())
	}
	return entries.Err()
}
