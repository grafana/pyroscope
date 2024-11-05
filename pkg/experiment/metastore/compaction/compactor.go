package compaction

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type Compactor struct {
	strategy strategy
	queue    *queue
	store    BlockQueueStore
}

func NewCompactor(store BlockQueueStore) *Compactor {
	c := Compactor{
		strategy: defaultCompactionStrategy,
		store:    store,
	}
	c.queue = newQueue(c.strategy)
	return &c
}

func (p *Compactor) CompactBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
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
	if p.queue.enqueue(c, b) {
		// Another entry with the same block ID already exists.
		// TODO: Add a log message, bump a metric, etc.
	}
}

func (p *Compactor) Restore(tx *bbolt.Tx) error {
	p.reset()
	entries := p.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		p.enqueue(entries.At())
	}
	return entries.Err()
}

func (p *Compactor) reset() {
	// Reset in-memory state before loading entries from the store.
	p.queue = newQueue(p.strategy)
}

func (p *Compactor) Planner(index PlannerIndexReader) *Planner {
	return NewPlanner(p, index)
}

func (p *Compactor) PlanUpdater(index IndexWriter) *PlanUpdater {
	return NewUpdater(p, index, p.store)
}
