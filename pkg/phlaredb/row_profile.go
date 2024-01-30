package phlaredb

import (
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
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

// The size of the batch is chosen empirically.
const profileRowAsyncBatchSize = 1 << 10

func profileRowBatchIterator(it iter.Iterator[*query.IteratorResult]) iter.Iterator[rowProfile] {
	return iter.NewAsyncBatchIterator[*query.IteratorResult, rowProfile](
		it, profileRowAsyncBatchSize,
		func(r *query.IteratorResult) rowProfile {
			return rowProfile{
				rowNum:    r.RowNumber[0],
				partition: r.ColumnValue(schemav1.StacktracePartitionColumnName).Uint64(),
			}
		},
		func(t []rowProfile) {},
	)
}

func profileBatchIteratorBySeriesIndex(
	it iter.Iterator[*query.IteratorResult],
	series map[int64]labelsInfo,
) iter.Iterator[Profile] {
	buf := make([][]parquet.Value, 3)
	return iter.NewAsyncBatchIterator[*query.IteratorResult, Profile](
		it, profileRowAsyncBatchSize,
		func(r *query.IteratorResult) Profile {
			buf = r.Columns(buf,
				schemav1.SeriesIndexColumnName,
				schemav1.TimeNanosColumnName,
				schemav1.StacktracePartitionColumnName)
			x := series[buf[0][0].Int64()]
			return BlockProfile{
				rowNum:      r.RowNumber[0],
				timestamp:   model.TimeFromUnixNano(buf[1][0].Int64()),
				partition:   retrieveStacktracePartition(buf, 2),
				fingerprint: x.fp,
				labels:      x.lbs,
			}
		},
		func(t []Profile) {},
	)
}

func profileBatchIteratorByFingerprints(
	it iter.Iterator[*query.IteratorResult],
	labels map[model.Fingerprint]phlaremodel.Labels,
) iter.Iterator[Profile] {
	return iter.NewAsyncBatchIterator[*query.IteratorResult, Profile](
		it, profileRowAsyncBatchSize,
		func(r *query.IteratorResult) Profile {
			v, ok := r.Entries[0].RowValue.(fingerprintWithRowNum)
			if !ok {
				panic("no fingerprint information found")
			}
			l, ok := labels[v.fp]
			if !ok {
				panic("no profile series labels with matching fingerprint found")
			}
			return BlockProfile{
				rowNum:      r.RowNumber[0],
				timestamp:   model.TimeFromUnixNano(r.ColumnValue(schemav1.TimeNanosColumnName).Int64()),
				partition:   r.ColumnValue(schemav1.StacktracePartitionColumnName).Uint64(),
				fingerprint: v.fp,
				labels:      l,
			}
		},
		func(t []Profile) {},
	)
}
