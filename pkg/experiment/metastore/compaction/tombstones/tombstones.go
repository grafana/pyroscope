package tombstones

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

type TombstoneStoreInterface interface {
	//  AddTombstones(*bbolt.Tx, *raft.Log, *metastorev1.Tombstones) error
	GetExpiredTombstones(*bbolt.Tx, *raft.Log) iter.Iterator[*metastorev1.Tombstones]
	//	DeleteTombstones(*bbolt.Tx, *raft.Log, ...*metastorev1.Tombstones) error
	//	ListEntries(*bbolt.Tx) iter.Iterator[*metastorev1.Tombstones]
}
