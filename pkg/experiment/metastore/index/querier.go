package index

import (
	"context"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type QuerierStore interface {
	ReadTx() *bbolt.Tx
}

type Querier struct {
	store QuerierStore
	index *Index // Might be an interface.
}

func NewQuerier(store QuerierStore, index *Index) *Querier {
	return &Querier{
		index: index,
		store: store,
	}
}

func (q *Querier) FindBlocksInRange(start, end int64, tenants map[string]struct{}) []*metastorev1.BlockMeta {
	tx := q.store.ReadTx()
	defer func() {
		_ = tx.Rollback()
	}()
	return q.index.FindBlocksInRange(tx, start, end, tenants)
}

func (q *Querier) ForEachPartition(ctx context.Context, fn func(*PartitionMeta) error) error {
	return q.index.ForEachPartition(ctx, fn)
}
