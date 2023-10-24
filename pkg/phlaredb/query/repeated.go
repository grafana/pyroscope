package query

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/parquet-go/parquet-go"
	"github.com/samber/lo"

	"github.com/grafana/pyroscope/pkg/iter"
)

type RepeatedRow[T any] struct {
	Row    T
	Values []parquet.Value
}

type repeatedPageIterator[T any] struct {
	rows     iter.Iterator[T]
	column   int
	readSize int
	ctx      context.Context
	span     opentracing.Span

	rgs                 []parquet.RowGroup
	startRowGroupRowNum int64

	currentPage     parquet.Page
	startPageRowNum int64

	pageNextRowNum int64

	currentPages parquet.Pages
	valueReader  parquet.ValueReader

	rowFinished    bool
	skipping       bool
	err            error
	done           bool // because we advance the iterator to seek in advance we remember if we are done
	currentValue   *RepeatedRow[T]
	buffer         []parquet.Value
	originalBuffer []parquet.Value
}

// NewRepeatedPageIterator returns an iterator that iterates over the repeated values in a column.
// The iterator can only seek forward and so rows should be sorted by row number.
func NewRepeatedPageIterator[T any](
	ctx context.Context,
	rows iter.Iterator[T],
	rgs []parquet.RowGroup,
	column int,
	readSize int,
) iter.Iterator[*RepeatedRow[T]] {
	if readSize <= 0 {
		panic("readSize must be greater than 0")
	}
	buffer := make([]parquet.Value, readSize)
	done := !rows.Next()
	span, ctx := opentracing.StartSpanFromContext(ctx, "NewRepeatedPageIterator")
	return &repeatedPageIterator[T]{
		ctx:            ctx,
		span:           span,
		rows:           rows,
		rgs:            rgs,
		column:         column,
		readSize:       readSize,
		buffer:         buffer[:0],
		originalBuffer: buffer,
		currentValue:   &RepeatedRow[T]{},
		done:           done,
		rowFinished:    true,
		skipping:       false,
	}
}

// seekRowNum the row num to seek to.
func (it *repeatedPageIterator[T]) seekRowNum() int64 {
	switch i := any(it.rows.At()).(type) {
	case RowGetter:
		return i.RowNumber()
	case int64:
		return i
	case uint32:
		return int64(i)
	default:
		panic(fmt.Sprintf("unknown row type: %T", it.rows.At()))
	}
}

func (it *repeatedPageIterator[T]) Next() bool {
Outer:
	for {
		if it.done {
			return false
		}
		for len(it.rgs) != 0 && (it.seekRowNum() >= (it.startRowGroupRowNum + it.rgs[0].NumRows())) {
			if !it.closeCurrentPages() {
				return false
			}
			it.startRowGroupRowNum += it.rgs[0].NumRows()
			it.rgs = it.rgs[1:]
		}
		if len(it.rgs) == 0 {
			return false
		}
		if it.currentPages == nil {
			it.currentPages = it.rgs[0].ColumnChunks()[it.column].Pages()
		}
		// read a new page.
		if it.currentPage == nil {
			// SeekToRow seek across and within pages. So the next position in the page will the be the row.
			seekTo := it.seekRowNum() - it.startRowGroupRowNum
			if err := it.currentPages.SeekToRow(seekTo); err != nil {
				it.err = err
				it.currentPages = nil // we can set it to nil since somehow it was closed.
				return false
			}
			it.startPageRowNum = it.seekRowNum()
			it.pageNextRowNum = 0
			it.buffer = it.buffer[:0]
			it.rowFinished = true
			it.skipping = false
			var err error
			pageReadStart := time.Now()
			it.currentPage, err = it.currentPages.ReadPage()
			pageReadDurationMs := time.Since(pageReadStart).Milliseconds()
			if err != nil {
				if err == io.EOF {
					continue
				}
				it.err = err
				return false
			}
			it.span.LogFields(
				otlog.String("msg", "Page read"),
				otlog.Int64("startRowGroupRowNum", it.startRowGroupRowNum),
				otlog.Int64("startPageRowNum", it.startPageRowNum),
				otlog.Int64("pageRowNum", it.currentPage.NumRows()),
				otlog.Int64("duration_ms", pageReadDurationMs),
			)
			it.valueReader = it.currentPage.Values()
		}
		// if there's no more value in that page we can skip it.
		if it.seekRowNum() >= it.startPageRowNum+it.currentPage.NumRows() {
			it.currentPage = nil
			continue
		}

		// only read values if the buffer is empty
		if len(it.buffer) == 0 {
			// reading values....
			it.buffer = it.originalBuffer
			n, err := it.valueReader.ReadValues(it.buffer)
			if err != nil && err != io.EOF {
				it.err = err
				return false
			}
			it.buffer = it.buffer[:n]
			// no more buffer, move to next page
			if len(it.buffer) == 0 {
				it.done = !it.rows.Next() // if the page has no more data the current row is over.
				it.currentPage = nil
				continue
			}
		}

		// we have data in the buffer.
		it.currentValue.Row = it.rows.At()
		start, next, ok := it.readNextRow()
		if ok && it.rowFinished {
			if it.seekRowNum() > it.startPageRowNum+it.pageNextRowNum {
				it.pageNextRowNum++
				it.buffer = it.buffer[next:]
				continue Outer
			}
			it.pageNextRowNum++
			it.currentValue.Values = it.buffer[:next]
			it.buffer = it.buffer[next:] // consume the values.
			it.done = !it.rows.Next()
			return true
		}
		// we read a partial row or we're skipping a row.
		if it.rowFinished || it.skipping {
			it.rowFinished = false
			// skip until we find the next row.
			if it.seekRowNum() > it.startPageRowNum+it.pageNextRowNum {
				last := it.buffer[start].RepetitionLevel()
				if it.skipping && last == 0 {
					it.buffer = it.buffer[start:]
					it.pageNextRowNum++
					it.skipping = false
					it.rowFinished = true
				} else {
					if start != 0 {
						next = start + 1
					}
					it.buffer = it.buffer[next:]
					it.skipping = true
				}
				continue Outer
			}
			it.currentValue.Values = it.buffer[:next]
			it.buffer = it.buffer[next:] // consume the values.
			return true
		}
		// this is the start of a new row.
		if !it.rowFinished && it.buffer[start].RepetitionLevel() == 0 {
			// consume values up to the new start if there is
			if start >= 1 {
				it.currentValue.Values = it.buffer[:start]
				it.buffer = it.buffer[start:] // consume the values.
				return true
			}
			// or move to the next row.
			it.pageNextRowNum++
			it.done = !it.rows.Next()
			it.rowFinished = true
			continue Outer
		}
		it.currentValue.Values = it.buffer[:next]
		it.buffer = it.buffer[next:] // consume the values.
		return true
	}
}

func (it *repeatedPageIterator[T]) readNextRow() (int, int, bool) {
	start := 0
	foundStart := false
	for i, v := range it.buffer {
		if v.RepetitionLevel() == 0 && !foundStart {
			foundStart = true
			start = i
			continue
		}
		if v.RepetitionLevel() == 0 && foundStart {
			return start, i, true
		}
	}
	return start, len(it.buffer), false
}

func (it *repeatedPageIterator[T]) closeCurrentPages() bool {
	if it.currentPages != nil {
		if err := it.currentPages.Close(); err != nil {
			it.err = err
			it.currentPages = nil
			return false
		}
		it.currentPages = nil
	}
	return true
}

// At returns the current value.
// Only valid after a call to Next.
// The returned value is reused on the next call to Next and should not be retained.
func (it *repeatedPageIterator[T]) At() *RepeatedRow[T] {
	return it.currentValue
}

func (it *repeatedPageIterator[T]) Err() error {
	return it.err
}

func (it *repeatedPageIterator[T]) Close() error {
	defer it.span.Finish()
	if it.currentPages != nil {
		if err := it.currentPages.Close(); err != nil {
			return err
		}
		it.currentPages = nil
	}
	return nil
}

type MultiRepeatedRow[T any] struct {
	Row    T
	Values [][]parquet.Value
}

type multiRepeatedPageIterator[T any] struct {
	iters     []iter.Iterator[*RepeatedRow[T]]
	asyncNext []<-chan bool
	err       error
	curr      *MultiRepeatedRow[T]
}

// NewMultiRepeatedPageIterator returns an iterator that iterates over the values of repeated columns nested together.
// Each column is iterate over in parallel.
// If one column is finished, the iterator will return false.
func NewMultiRepeatedPageIterator[T any](iters ...iter.Iterator[*RepeatedRow[T]]) iter.Iterator[*MultiRepeatedRow[T]] {
	return &multiRepeatedPageIterator[T]{
		iters:     iters,
		asyncNext: make([]<-chan bool, len(iters)),
		curr: &MultiRepeatedRow[T]{
			Values: make([][]parquet.Value, len(iters)),
		},
	}
}

func (it *multiRepeatedPageIterator[T]) Next() bool {
	for i := range it.iters {
		i := i
		it.asyncNext[i] = lo.Async(func() bool {
			next := it.iters[i].Next()
			if next {
				it.curr.Values[i] = it.iters[i].At().Values
				if i == 0 {
					it.curr.Row = it.iters[i].At().Row
				}
			}
			return next
		})
	}
	next := true
	for i := range it.iters {
		if !<-it.asyncNext[i] {
			next = false
		}
	}
	return next
}

func (it *multiRepeatedPageIterator[T]) At() *MultiRepeatedRow[T] {
	return it.curr
}

func (it *multiRepeatedPageIterator[T]) Err() error {
	errs := multierror.New()
	errs.Add(it.err)
	for _, i := range it.iters {
		errs.Add(i.Err())
	}
	return errs.Err()
}

func (it *multiRepeatedPageIterator[T]) Close() error {
	errs := multierror.New()
	for _, i := range it.iters {
		errs.Add(i.Close())
	}
	return errs.Err()
}

type rowIterator[T any] struct {
	columns iter.Iterator[[][]parquet.Value]
	rows    iter.Iterator[T]
}

func NewRowIterator[T any](
	rows iter.Iterator[T],
	rowGroups []parquet.RowGroup,
	columns ...int,
) iter.Iterator[*MultiRepeatedRow[T]] {
	rows, rowNumbers := iter.Tee(rows)
	return &rowIterator[T]{
		columns: NewMultiColumnIterator(WrapWithRowNumber(rowNumbers), 1<<10, rowGroups, columns...),
		rows:    rows,
	}
}

func (x *rowIterator[T]) Next() bool {
	if !x.rows.Next() {
		return false
	}
	return x.columns.Next()
}

func (x *rowIterator[T]) At() *MultiRepeatedRow[T] {
	return &MultiRepeatedRow[T]{
		Values: x.columns.At(),
		Row:    x.rows.At(),
	}
}

func (x *rowIterator[T]) Err() error {
	return x.columns.Err()
}

func (x *rowIterator[T]) Close() error {
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
	r := iter.TeeN(rows, len(columns))
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
