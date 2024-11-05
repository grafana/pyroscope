package compactor

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/iter"
)

var _ compaction.Planner = (*Planner)(nil)

type PlannerIndex interface {
	PlannerIndexReader
	PlannerIndexWriter
}

type PlannerIndexReader interface {
	LookupBlocks(tx *bbolt.Tx, tenant string, shard uint32, blocks []string) []*metastorev1.BlockMeta
	GetTombstones(tx *bbolt.Tx, tenant string, shard uint32) []string
}

type PlannerIndexWriter interface {
	Replace(*bbolt.Tx, *raft_log.CompactedBlocks) error
	DeleteTombstones(*bbolt.Tx, []string) error
}

type BlockQueueStore interface {
	StoreEntry(*bbolt.Tx, BlockEntry) error
	ListEntries(*bbolt.Tx) iter.Iterator[BlockEntry]
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error
}

type BlockEntry struct {
	Index  uint64
	ID     string
	Shard  uint32
	Level  uint32
	Tenant string
}

type Planner struct {
	strategy strategy
	index    PlannerIndex
	store    BlockQueueStore
	queue    *compactionQueue
}

func NewPlanner(index PlannerIndex, store BlockQueueStore) *Planner {
	config := defaultCompactionStrategy
	return &Planner{
		strategy: config,
		index:    index,
		store:    store,
		queue:    newCompactionQueue(config),
	}
}

func (p *Planner) AddBlocks(tx *bbolt.Tx, cmd *raft.Log, blocks ...*metastorev1.BlockMeta) error {
	for _, md := range blocks {
		if err := p.AddBlock(tx, cmd, md); err != nil {
			return err
		}
	}
	return nil
}

func (p *Planner) AddBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
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

func (p *Planner) Restore(tx *bbolt.Tx) error {
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

func (p *Planner) enqueue(e BlockEntry) {
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

func (p *Planner) NewPlan(tx *bbolt.Tx) compaction.Plan {
	return &compactionPlan{
		tx:       tx,
		index:    p.index,
		strategy: p.strategy,
		queue:    p.queue,
		blocks:   newBlockIter(),
	}
}

func (p *Planner) Planned(tx *bbolt.Tx, job *metastorev1.CompactionJob) error {
	k := compactionKey{
		tenant: job.Tenant,
		shard:  job.Shard,
		level:  job.CompactionLevel,
	}
	staged := p.queue.stagedBlocks(k)
	for _, block := range job.SourceBlocks {
		e := staged.delete(block.Id)
		if e == zeroBlockEntry {
			continue
		}
		if err := p.store.DeleteEntry(tx, e.raftIndex, e.id); err != nil {
			return err
		}
	}
	return nil
}

func (p *Planner) Compacted(tx *bbolt.Tx, compacted *raft_log.CompactedBlocks) error {
	if err := p.index.Replace(tx, compacted); err != nil {
		return err
	}
	return p.index.DeleteTombstones(tx, compacted.DeletedBlocks)
}
