package query

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/phlare/pkg/iter"
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
	asyncNext []chan bool
	err       error
	curr      *MultiRepeatedRow[T]
}

// NewMultiRepeatedPageIterator returns an iterator that iterates over the values of repeated columns nested together.
// Each column is iterate over in parallel.
// If one column is finished, the iterator will return false.
func NewMultiRepeatedPageIterator[T any](iters ...iter.Iterator[*RepeatedRow[T]]) iter.Iterator[*MultiRepeatedRow[T]] {
	return &multiRepeatedPageIterator[T]{
		iters:     iters,
		asyncNext: make([]chan bool, len(iters)),
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
