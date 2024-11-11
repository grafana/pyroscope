package store

import (
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/iter"
)

type BlockEntry struct {
	Index      uint64
	AppendedAt int64
	ID         string
	Tenant     string
	Shard      uint32
	Level      uint32
}

type BlockQueueStore struct{}

func (b BlockQueueStore) StoreEntry(tx *bbolt.Tx, entry BlockEntry) error {
	//TODO implement me
	panic("implement me")
}

func (b BlockQueueStore) DeleteEntry(tx *bbolt.Tx, index uint64, id string) error {
	//TODO implement me
	panic("implement me")
}

func (b BlockQueueStore) ListEntries(tx *bbolt.Tx) iter.Iterator[BlockEntry] {
	//TODO implement me
	panic("implement me")
}
