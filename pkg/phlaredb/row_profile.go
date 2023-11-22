package phlaredb

import (
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	"github.com/prometheus/common/model"
)

type labelsInfo struct {
	fp  model.Fingerprint
	lbs phlaremodel.Labels
}

type rowProfile struct {
	rowNum    int64
	partition uint64
}

func (p rowProfile) StacktracePartition() uint64 {
	return p.partition
}

func (p rowProfile) RowNumber() int64 {
	return p.rowNum
}

type RowsIterator[T any] struct {
	rows    query.Iterator
	current T
	at      func(*query.IteratorResult) T
}

func (it *RowsIterator[T]) Next() bool {
	if it.rows.Next() {
		it.current = it.at(it.rows.At())
		return true
	}
	return false
}

func (it *RowsIterator[T]) Close() error {
	return it.rows.Close()
}

func (it *RowsIterator[T]) Err() error {
	return it.rows.Err()
}

func (it *RowsIterator[T]) At() T {
	return it.current
}
