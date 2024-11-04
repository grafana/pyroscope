package compaction

import (
	"encoding/binary"

	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/iter"
)

type Store interface {
	StoreEntry(*bbolt.Tx, BlockEntry) error
	ListEntries(*bbolt.Tx) iter.Iterator[BlockEntry]
	DeleteEntry(tx *bbolt.Tx, index uint64, id string) error

	StoreJob(*bbolt.Tx, any) error
	GetJob(*bbolt.Tx, string) (any, error)
	DeleteJob(*bbolt.Tx, string) error

	UpdateJobState(*bbolt.Tx, *raft_log.CompactionJobState) error
	DeleteJobState(*bbolt.Tx, string) error
}

type BlockEntry struct {
	Index  uint64
	ID     string
	Shard  uint32
	Level  uint32
	Tenant string
}

func (e *BlockEntry) Key() []byte {
	k := make([]byte, 8+len(e.ID))
	binary.LittleEndian.PutUint64(k[:8], e.Index)
	copy(k[8:], e.ID)
	return k
}

func (e *BlockEntry) Value() []byte {
	v := make([]byte, 8+4+4+len(e.Tenant))
	binary.LittleEndian.AppendUint64(v[0:8], e.Index)
	binary.LittleEndian.PutUint32(v[8:12], e.Level)
	binary.LittleEndian.PutUint32(v[12:16], e.Shard)
	copy(v[16:], e.Tenant)
	return v
}
