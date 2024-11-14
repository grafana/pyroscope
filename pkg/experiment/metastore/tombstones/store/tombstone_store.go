package store

import (
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

// index:appended_at     *metastorev1.Tombstones

type TombstoneEntry struct {
	Index      uint64
	AppendedAt int64
	*metastorev1.Tombstones
}

type TombstoneStore struct {
}

func (s TombstoneStore) StoreTombstones(tx *bbolt.Tx, entry TombstoneEntry) error {
	//TODO implement me
	panic("implement me")
}

func (s TombstoneStore) DeleteTombstones(tx *bbolt.Tx, entry TombstoneEntry) error {
	//TODO implement me
	panic("implement me")
}

func (s TombstoneStore) ListEntries(tx *bbolt.Tx) iter.Iterator[TombstoneEntry] {
	//TODO implement me
	panic("implement me")
}

func NewTombstoneStore() *TombstoneStore {
	return &TombstoneStore{}
}
