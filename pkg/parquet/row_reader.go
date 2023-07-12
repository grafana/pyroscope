package parquet

import (
	"io"

	"github.com/grafana/dskit/runutil"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/phlare/pkg/iter"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/loser"
)

const (
	defaultRowBufferSize = 1024
)

var (
	_ parquet.RowReader          = (*emptyRowReader)(nil)
	_ parquet.RowReader          = (*ErrRowReader)(nil)
	_ parquet.RowReader          = (*IteratorRowReader)(nil)
	_ iter.Iterator[parquet.Row] = (*BufferedRowReaderIterator)(nil)

	EmptyRowReader = &emptyRowReader{}
)

type emptyRowReader struct{}

func (e *emptyRowReader) ReadRows(rows []parquet.Row) (int, error) { return 0, io.EOF }

type ErrRowReader struct{ err error }

func NewErrRowReader(err error) *ErrRowReader { return &ErrRowReader{err: err} }

func (e ErrRowReader) ReadRows(rows []parquet.Row) (int, error) { return 0, e.err }

// NewMergeRowReader returns a RowReader that k-way merges the given readers using the less function.
// Each reader must be sorted according to the less function already.
func NewMergeRowReader(readers []parquet.RowReader, maxValue parquet.Row, less func(parquet.Row, parquet.Row) bool) parquet.RowReader {
	if len(readers) == 0 {
		return EmptyRowReader
	}
	if len(readers) == 1 {
		return readers[0]
	}
	its := make([]iter.Iterator[parquet.Row], len(readers))
	for i := range readers {
		its[i] = NewBufferedRowReaderIterator(readers[i], defaultRowBufferSize)
	}

	return NewIteratorRowReader(
		iter.NewTreeIterator[parquet.Row](
			loser.New(
				its,
				maxValue,
				func(it iter.Iterator[parquet.Row]) parquet.Row { return it.At() },
				less,
				func(it iter.Iterator[parquet.Row]) { _ = it.Close() },
			),
		),
	)
}

type IteratorRowReader struct {
	iter.Iterator[parquet.Row]
}

// NewIteratorRowReader returns a RowReader that reads rows from the given iterator.
func NewIteratorRowReader(it iter.Iterator[parquet.Row]) *IteratorRowReader {
	return &IteratorRowReader{
		Iterator: it,
	}
}

func (it *IteratorRowReader) ReadRows(rows []parquet.Row) (int, error) {
	var n int
	if len(rows) == 0 {
		return 0, nil
	}
	for {
		if n == len(rows) {
			break
		}
		if !it.Next() {
			runutil.CloseWithLogOnErr(util.Logger, it.Iterator, "failed to close iterator")
			if it.Err() != nil {
				return n, it.Err()
			}
			return n, io.EOF
		}
		rows[n] = rows[n][:0]
		for _, c := range it.At() {
			rows[n] = append(rows[n], c.Clone())
		}
		n++
	}
	return n, nil
}

type BufferedRowReaderIterator struct {
	reader       parquet.RowReader
	bufferedRows []parquet.Row

	// buff keep the original slice capacity to avoid allocations
	buff       []parquet.Row
	bufferSize int
	err        error
}

// NewBufferedRowReaderIterator returns a new `iter.Iterator[parquet.Row]` from a RowReader.
// The iterator will buffer `bufferSize` rows from the reader.
func NewBufferedRowReaderIterator(reader parquet.RowReader, bufferSize int) *BufferedRowReaderIterator {
	return &BufferedRowReaderIterator{
		reader:     reader,
		bufferSize: bufferSize,
	}
}

func (r *BufferedRowReaderIterator) Next() bool {
	if len(r.bufferedRows) > 1 {
		r.bufferedRows = r.bufferedRows[1:]
		return true
	}

	// todo this seems to do allocations on every call since cap is always smaller
	if cap(r.buff) < r.bufferSize {
		r.buff = make([]parquet.Row, r.bufferSize)
	}
	r.bufferedRows = r.buff[:r.bufferSize]
	n, err := r.reader.ReadRows(r.bufferedRows)
	if err != nil && err != io.EOF {
		r.err = err
		return false
	}
	if n == 0 {
		return false
	}

	r.bufferedRows = r.bufferedRows[:n]
	return true
}

func (r *BufferedRowReaderIterator) At() parquet.Row {
	if len(r.bufferedRows) == 0 {
		return parquet.Row{}
	}
	return r.bufferedRows[0]
}

func (r *BufferedRowReaderIterator) Err() error {
	return r.err
}

func (r *BufferedRowReaderIterator) Close() error {
	return r.err
}

func ReadAll(r parquet.RowReader) ([]parquet.Row, error) {
	return ReadAllWithBufferSize(r, defaultRowBufferSize)
}

func ReadAllWithBufferSize(r parquet.RowReader, bufferSize int) ([]parquet.Row, error) {
	var rows []parquet.Row
	batch := make([]parquet.Row, bufferSize)
	for {
		n, err := r.ReadRows(batch)
		if err != nil && err != io.EOF {
			return rows, err
		}
		if n != 0 {
			for i := range batch[:n] {
				rows = append(rows, batch[i].Clone())
			}
		}
		if n == 0 || err == io.EOF {
			break
		}
	}
	return rows, nil
}
