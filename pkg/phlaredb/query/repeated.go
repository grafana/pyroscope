package query

import (
	"fmt"
	"io"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"

	"github.com/grafana/pyroscope/pkg/iter"
)

type RepeatedRow[T any] struct {
	Row    T
	Values [][]parquet.Value
}

type repeatedRowIterator[T any] struct {
	columns iter.Iterator[[][]parquet.Value]
	rows    iter.Iterator[T]
}

// Batch size chosen empirically.
const defaultRepeatedRowIteratorBatchSize = 128

func NewRepeatedRowIterator[T any](
	rows iter.Iterator[T],
	rowGroups []parquet.RowGroup,
	columns ...int,
) iter.Iterator[RepeatedRow[T]] {
	rows, rowNumbers := iter.Tee(rows)
	return &repeatedRowIterator[T]{
		rows: rows,
		columns: NewMultiColumnIterator(
			WrapWithRowNumber(rowNumbers),
			defaultRepeatedRowIteratorBatchSize,
			rowGroups, columns...),
	}
}

func (x *repeatedRowIterator[T]) Next() bool {
	if !x.rows.Next() {
		return false
	}
	return x.columns.Next()
}

func (x *repeatedRowIterator[T]) At() RepeatedRow[T] {
	return RepeatedRow[T]{
		Values: x.columns.At(),
		Row:    x.rows.At(),
	}
}

func (x *repeatedRowIterator[T]) Err() error {
	return x.columns.Err()
}

func (x *repeatedRowIterator[T]) Close() error {
	return x.columns.Close()
}

type rowNumberIterator[T any] struct{ it iter.Iterator[T] }

func WrapWithRowNumber[T any](it iter.Iterator[T]) iter.Iterator[int64] {
	return &rowNumberIterator[T]{it}
}

func (x *rowNumberIterator[T]) Next() bool   { return x.it.Next() }
func (x *rowNumberIterator[T]) Err() error   { return x.it.Err() }
func (x *rowNumberIterator[T]) Close() error { return x.it.Close() }

func (x *rowNumberIterator[T]) At() int64 {
	v := any(x.it.At())
	switch r := v.(type) {
	case RowGetter:
		return r.RowNumber()
	case int64:
		return r
	case uint32:
		return int64(r)
	default:
		panic(fmt.Sprintf("unknown row type: %T", v))
	}
}

type multiColumnIterator struct {
	it []iter.Iterator[[]parquet.Value]
	v  [][]parquet.Value
}

func NewMultiColumnIterator(
	rows iter.Iterator[int64],
	batchSize int,
	rowGroups []parquet.RowGroup,
	columns ...int,
) iter.Iterator[[][]parquet.Value] {
	m := multiColumnIterator{
		it: make([]iter.Iterator[[]parquet.Value], len(columns)),
		v:  make([][]parquet.Value, len(columns)),
	}
	// FIXME(kolesnikovae): r := iter.TeeN(rows, len(columns))
	r, err := iter.CloneN(rows, len(columns))
	if err != nil {
		return iter.NewErrIterator[[][]parquet.Value](err)
	}
	for i, column := range columns {
		m.it[i] = iter.NewAsyncBatchIterator[[]parquet.Value](
			NewRepeatedColumnIterator(r[i], rowGroups, column),
			batchSize,
			CloneParquetValues,
			ReleaseParquetValues,
		)
	}
	return &m
}

func (m *multiColumnIterator) Next() bool {
	for i, x := range m.it {
		if !x.Next() {
			return false
		}
		m.v[i] = x.At()
	}
	return true
}

func (m *multiColumnIterator) At() [][]parquet.Value { return m.v }

func (m *multiColumnIterator) Err() error {
	var err multierror.MultiError
	for _, x := range m.it {
		err.Add(x.Err())
	}
	return err.Err()
}

func (m *multiColumnIterator) Close() error {
	var err multierror.MultiError
	for _, x := range m.it {
		err.Add(x.Close())
	}
	return err.Err()
}

var ErrSeekOutOfRange = fmt.Errorf("bug: south row is out of range")

type repeatedColumnIterator struct {
	rows     iter.Iterator[int64]
	rgs      []parquet.RowGroup
	column   int
	readSize int

	pages parquet.Pages
	page  parquet.Page

	minRGRowNum   int64
	maxRGRowNum   int64
	maxPageRowNum int64

	vit  *repeatedValuePageIterator
	prev int64
	err  error
}

// Too big read size does not make much sense: despite
// the fact that this does not impact read amplification
// as the page is already fully read, decoding of the
// values is not free.
//
// How many values we expect per a row, the upper boundary?
const repeatedColumnIteratorReadSize = 2 << 10

func NewRepeatedColumnIterator(rows iter.Iterator[int64], rgs []parquet.RowGroup, column int) iter.Iterator[[]parquet.Value] {
	return &repeatedColumnIterator{
		rows:     rows,
		rgs:      rgs,
		column:   column,
		vit:      getRepeatedValuePageIteratorFromPool(),
		readSize: repeatedColumnIteratorReadSize,
	}
}

func (x *repeatedColumnIterator) Next() bool {
	if !x.rows.Next() || x.err != nil {
		return false
	}
	rn := x.rows.At()
	if rn >= x.maxRGRowNum {
		if !x.seekRowGroup(rn) {
			return false
		}
	}
	rn -= x.minRGRowNum
	if x.page == nil || rn >= x.maxPageRowNum {
		if !x.readPage(rn) {
			return false
		}
		// readPage ensures that the first row in the
		// page matches rn, therefore we don't need to
		// skip anything.
		x.prev = rn - 1
	}
	// Skip rows to the rn.
	next := int(rn - x.prev)
	x.prev = rn
	for i := 0; i < next; i++ {
		if !x.vit.Next() {
			x.err = ErrSeekOutOfRange
			return false
		}
	}
	return true
}

func (x *repeatedColumnIterator) seekRowGroup(rn int64) bool {
	for i, rg := range x.rgs {
		x.minRGRowNum = x.maxRGRowNum
		x.maxRGRowNum += rg.NumRows()
		if rn >= x.maxRGRowNum {
			continue
		}
		x.rgs = x.rgs[i+1:]
		return x.openChunk(rg)
	}
	return false
}

func (x *repeatedColumnIterator) openChunk(rg parquet.RowGroup) bool {
	x.page = nil
	x.vit.reset(nil, 0)
	if x.pages != nil {
		if x.err = x.pages.Close(); x.err != nil {
			return false
		}
	}
	x.pages = rg.ColumnChunks()[x.column].Pages()
	return true
}

func (x *repeatedColumnIterator) readPage(rn int64) bool {
	if x.err = x.pages.SeekToRow(rn); x.err != nil {
		return false
	}
	if x.page, x.err = x.pages.ReadPage(); x.err != nil {
		if x.err != io.EOF {
			return false
		}
		x.err = nil
		// ReadPage should never return page along with EOF,
		// however this is not a strict contract.
		if x.page == nil {
			return false
		}
	}
	// NumRows return the number of row in the page
	// not counting skipped ones (because of SeekToRow).
	// The implementation is quite expensive, therefore
	// we should call it once per page.
	x.maxPageRowNum = rn + x.page.NumRows()
	x.vit.reset(x.page, x.readSize)
	return true
}

func (x *repeatedColumnIterator) At() []parquet.Value { return x.vit.At() }
func (x *repeatedColumnIterator) Err() error          { return x.err }
func (x *repeatedColumnIterator) Close() error {
	putRepeatedValuePageIteratorToPool(x.vit)
	return x.pages.Close()
}

var repeatedValuePageIteratorPool = sync.Pool{New: func() any { return new(repeatedValuePageIterator) }}

func getRepeatedValuePageIteratorFromPool() *repeatedValuePageIterator {
	return repeatedValuePageIteratorPool.Get().(*repeatedValuePageIterator)
}

func putRepeatedValuePageIteratorToPool(x *repeatedValuePageIterator) {
	x.reset(nil, 0)
	repeatedValuePageIteratorPool.Put(x)
}

// RepeatedValuePageIterator iterates over repeated fields.
// FIXME(kolesnikovae): Definition level is ignored.
type repeatedValuePageIterator struct {
	page parquet.ValueReader
	buf  []parquet.Value
	off  int
	row  []parquet.Value
	err  error
}

func NewRepeatedValuePageIterator(page parquet.Page, readSize int) iter.Iterator[[]parquet.Value] {
	var r repeatedValuePageIterator
	r.reset(page, readSize)
	return &r
}

func (x *repeatedValuePageIterator) At() []parquet.Value { return x.row }
func (x *repeatedValuePageIterator) Err() error          { return x.err }
func (x *repeatedValuePageIterator) Close() error        { return nil }

func (x *repeatedValuePageIterator) reset(page parquet.Page, readSize int) {
	if cap(x.buf) < readSize {
		x.buf = make([]parquet.Value, 0, readSize)
	}
	x.page = nil
	if page != nil {
		x.page = page.Values()
	}
	x.buf = x.buf[:0]
	x.row = x.row[:0]
	x.err = nil
	x.off = 0
}

func (x *repeatedValuePageIterator) Next() bool {
	if x.err != nil {
		return false
	}
	x.row = x.row[:0]
	var err error
	var n int
loop:
	for {
		buf := x.buf[x.off:]
		for _, v := range buf {
			if v.RepetitionLevel() == 0 && len(x.row) > 0 {
				// Found a new row.
				break loop
			}
			x.row = append(x.row, v)
			x.off++
		}
		// Refill the buffer.
		x.buf = x.buf[:cap(x.buf)]
		x.off = 0
		n, err = x.page.ReadValues(x.buf)
		x.buf = x.buf[:n]
		if err != nil && err != io.EOF {
			x.err = err
		}
		if n == 0 {
			break
		}
	}
	return len(x.row) > 0
}
