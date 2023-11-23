package phlaredb

import (
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
)

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

// RowsIterator is an iterator over rows of a parquet table.
// It is a wrapper over query.Iterator to transform its results into a desired type.
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
