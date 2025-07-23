package tombstones

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/metastore/index/tombstones/store"
)

type TombstoneStore interface {
	StoreTombstones(*bbolt.Tx, store.TombstoneEntry) error
	DeleteTombstones(*bbolt.Tx, store.TombstoneEntry) error
	ListEntries(*bbolt.Tx) iter.Iterator[store.TombstoneEntry]
	CreateBuckets(*bbolt.Tx) error
}

type Tombstones struct {
	metrics    *metrics
	tombstones map[tombstoneKey]*tombstones
	blocks     map[tenantBlockKey]*tenantBlocks
	queue      *tombstoneQueue
	store      TombstoneStore
}

type tenantBlockKey struct {
	tenant string
	shard  uint32
}

type tenantBlocks struct {
	blocks map[string]struct{}
}

func NewTombstones(store TombstoneStore, reg prometheus.Registerer) *Tombstones {
	return &Tombstones{
		metrics:    newMetrics(reg),
		tombstones: make(map[tombstoneKey]*tombstones),
		blocks:     make(map[tenantBlockKey]*tenantBlocks),
		queue:      newTombstoneQueue(),
		store:      store,
	}
}

func NewStore() *store.TombstoneStore {
	return store.NewTombstoneStore()
}

func (x *Tombstones) Exists(tenant string, shard uint32, block string) bool {
	t, exists := x.blocks[tenantBlockKey{tenant: tenant, shard: shard}]
	if exists {
		_, exists = t.blocks[block]
	}
	return exists
}

func (x *Tombstones) ListTombstones(before time.Time) iter.Iterator[*metastorev1.Tombstones] {
	return &tombstoneIter{
		head:   x.queue.head,
		before: before.UnixNano(),
	}
}

func (x *Tombstones) AddTombstones(tx *bbolt.Tx, cmd *raft.Log, t *metastorev1.Tombstones) error {
	var k tombstoneKey
	if !k.set(t) {
		return nil
	}
	v := store.TombstoneEntry{
		Index:      cmd.Index,
		AppendedAt: cmd.AppendedAt.UnixNano(),
		Tombstones: t,
	}
	if !x.put(k, v) {
		return nil
	}
	return x.store.StoreTombstones(tx, v)
}

func (x *Tombstones) DeleteTombstones(tx *bbolt.Tx, cmd *raft.Log, tombstones ...*metastorev1.Tombstones) error {
	for _, t := range tombstones {
		if err := x.deleteTombstones(tx, cmd, t); err != nil {
			return err
		}
	}
	return nil
}

func (x *Tombstones) deleteTombstones(tx *bbolt.Tx, _ *raft.Log, t *metastorev1.Tombstones) error {
	var k tombstoneKey
	if !k.set(t) {
		return nil
	}
	e := x.delete(k)
	if e == nil {
		return nil
	}
	return x.store.DeleteTombstones(tx, e.TombstoneEntry)
}

func (x *Tombstones) put(k tombstoneKey, v store.TombstoneEntry) bool {
	if _, found := x.tombstones[k]; found {
		return false
	}
	e := &tombstones{TombstoneEntry: v}
	x.tombstones[k] = e
	x.queue.push(e)
	x.metrics.incrementTombstones(v.Tombstones)
	if v.Blocks != nil {
		// Keep track of the blocks we enqueued. This is
		// necessary to answer if the block has already
		// been deleted (within a limited time window).
		x.putBlockTombstones(v.Blocks)
	}
	return true
}

func (x *Tombstones) delete(k tombstoneKey) (t *tombstones) {
	e, found := x.tombstones[k]
	if !found {
		return nil
	}
	delete(x.tombstones, k)
	x.metrics.decrementTombstones(e.Tombstones)
	if t = x.queue.delete(e); t != nil {
		if t.Blocks != nil {
			x.deleteBlockTombstones(t.Blocks)
		}
	}
	return t
}

func (x *Tombstones) putBlockTombstones(t *metastorev1.BlockTombstones) {
	bk := tenantBlockKey{
		tenant: t.Tenant,
		shard:  t.Shard,
	}
	m, ok := x.blocks[bk]
	if !ok {
		m = &tenantBlocks{blocks: make(map[string]struct{})}
		x.blocks[bk] = m
	}
	for _, b := range t.Blocks {
		m.blocks[b] = struct{}{}
	}
}

func (x *Tombstones) deleteBlockTombstones(t *metastorev1.BlockTombstones) {
	bk := tenantBlockKey{
		tenant: t.Tenant,
		shard:  t.Shard,
	}
	m, found := x.blocks[bk]
	if !found {
		return
	}
	for _, b := range t.Blocks {
		delete(m.blocks, b)
	}
}

func (x *Tombstones) Init(tx *bbolt.Tx) error {
	return x.store.CreateBuckets(tx)
}

func (x *Tombstones) Restore(tx *bbolt.Tx) error {
	x.queue = newTombstoneQueue()
	clear(x.tombstones)
	clear(x.blocks)
	x.metrics.tombstones.Reset()
	entries := x.store.ListEntries(tx)
	defer func() {
		_ = entries.Close()
	}()
	for entries.Next() {
		var k tombstoneKey
		if v := entries.At(); k.set(v.Tombstones) {
			x.put(k, v)
		}
	}
	return entries.Err()
}
