package tombstones

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

// Tombstones are named.
type tombstoneKey string

type tombstoneQueue struct {
	head, tail *tombstones
	// The map is used to quickly check
	// if a tombstone for a block exists.
	tombstones map[tombstoneKey]*tombstones
}

type tombstones struct {
	addedAt uint64

	*metastorev1.Tombstones
	next, prev *tombstones
}

func (q *tombstoneQueue) put(t *metastorev1.Tombstones) {
	if t.Blocks == nil {
		return
	}
	b := t.Blocks

	k := tombstoneKey{tenant: b.Tenant, shard: b.Shard}
	m, exists := q.tombstones[k]
	if !exists {
		m = &blockTombstones{blocks: make(map[string]*tombstones)}
		q.tombstones[k] = m
	}

}

func (q *tombstoneQueue) delete(t *metastorev1.Tombstones) {

}

type TombstoneStore struct{}

// added_at:tombstone_key

func (s *TombstoneStore) Exists(meta *metastorev1.BlockMeta) bool {
	//TODO implement me
	panic("implement me")
}

func (s *TombstoneStore) AddTombstones(tx *bbolt.Tx, log *raft.Log, tombstones *metastorev1.Tombstones) error {
	//TODO implement me
	panic("implement me")
}

func (s *TombstoneStore) GetExpiredTombstones(tx *bbolt.Tx, log *raft.Log) iter.Iterator[*metastorev1.Tombstones] {
	//TODO implement me
	panic("implement me")
}

func (s *TombstoneStore) DeleteTombstones(tx *bbolt.Tx, log *raft.Log, tombstones ...*metastorev1.Tombstones) error {
	//TODO implement me
	panic("implement me")
}

func (s *TombstoneStore) ListEntries(tx *bbolt.Tx) iter.Iterator[*metastorev1.Tombstones] {
	//TODO implement me
	panic("implement me")
}
