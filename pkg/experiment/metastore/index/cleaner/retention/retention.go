package retention

import (
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
)

// Policy determines which parts of the index should be retained or deleted.
type Policy interface {
	// Visit is given access to the index partitions.
	// The function is called for each partition in the index, in the order
	// of their creation. Once the function returns false, the iteration stops.
	Visit(*bbolt.Tx, indexstore.Partition) bool
	// Tombstones returns a list of tombstones to be created.
	// The method is only called after when the View method returns false.
	Tombstones() []*metastorev1.Tombstones
}
