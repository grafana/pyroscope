package store

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

type TombstoneStore struct{}

func (t TombstoneStore) Exists(meta *metastorev1.BlockMeta) bool {
	//TODO implement me
	panic("implement me")
}

func (t TombstoneStore) AddTombstones(tx *bbolt.Tx, log *raft.Log, tombstones *metastorev1.Tombstones) error {
	//TODO implement me
	panic("implement me")
}

func (t TombstoneStore) GetTombstones(tx *bbolt.Tx, log *raft.Log) (*metastorev1.Tombstones, error) {
	//TODO implement me
	panic("implement me")
}

func (t TombstoneStore) DeleteTombstones(tx *bbolt.Tx, log *raft.Log, tombstones *metastorev1.Tombstones) error {
	//TODO implement me
	panic("implement me")
}

func (t TombstoneStore) ListEntries(tx *bbolt.Tx) iter.Iterator[BlockEntry] {
	//TODO implement me
	panic("implement me")
}
