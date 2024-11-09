package compactor

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/iter"
)

var (
	_ compaction.Compactor = (*Compactor)(nil)
	_ compaction.Planner   = (*Compactor)(nil)
)

type Index interface {
	ListExpiredTombstones(*bbolt.Tx, *raft.Log) iter.Iterator[string]
}

type BlockQueueStore interface {
	StoreEntry(*bbolt.Tx, BlockEntry) error
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error
	ListEntries(*bbolt.Tx) iter.Iterator[BlockEntry]
}

type BlockEntry struct {
	Index  uint64
	ID     string
	Shard  uint32
	Level  uint32
	Tenant string
}

type Compactor struct {
	strategy strategy
	queue    *compactionQueue
	store    BlockQueueStore
	index    Index
}

func NewCompactor(store BlockQueueStore, index Index) *Compactor {
	config := defaultCompactionStrategy
	return &Compactor{
		strategy: config,
		queue:    newCompactionQueue(config),
		store:    store,
		index:    index,
	}
}

func (p *Compactor) AddBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error {
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

func (p *Compactor) enqueue(e BlockEntry) bool {
	c := compactionKey{
		tenant: e.Tenant,
		shard:  e.Shard,
		level:  e.Level,
	}
	b := blockEntry{
		raftIndex: e.Index,
		id:        e.ID,
	}
	return p.queue.push(c, b)
}

func (p *Compactor) NewPlan(tx *bbolt.Tx, cmd *raft.Log) compaction.Plan {
	return &plan{
		tx:        tx,
		cmd:       cmd,
		compactor: p,
		blocks:    newBlockIter(),
	}
}

func (p *Compactor) Scheduled(tx *bbolt.Tx, jobs ...*raft_log.CompactionJobUpdate) error {
	for _, job := range jobs {
		k := compactionKey{
			tenant: job.Plan.Tenant,
			shard:  job.Plan.Shard,
			level:  job.Plan.CompactionLevel,
		}
		staged := p.queue.stagedBlocks(k)
		for _, block := range job.Plan.SourceBlocks {
			e := staged.delete(block)
			if e == zeroBlockEntry {
				continue
			}
			if err := p.store.DeleteEntry(tx, e.raftIndex, e.id); err != nil {
				return err
			}
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
