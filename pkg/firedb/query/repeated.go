package query

import (
	"io"

	"github.com/segmentio/parquet-go"

	"github.com/grafana/fire/pkg/iter"
)

type RepeatedRow[T any] struct {
	Row    T
	Values []parquet.Value
}

type repeatedPageIterator[T any] struct {
	rows     iter.Iterator[T]
	column   int
	readSize int

	rgs                 []parquet.RowGroup
	startRowGroupRowNum int64

	currentPage     parquet.Page
	startPageRowNum int64

	currRowNum int64

	currentPages parquet.Pages
	valueReader  parquet.ValueReader

	err            error
	currentValue   RepeatedRow[T]
	buffer         []parquet.Value
	originalBuffer []parquet.Value
}

func NewRepeatedPageIterator[T any](
	rows iter.Iterator[T],
	rgs []parquet.RowGroup,
	column int,
	readSize int,
) iter.Iterator[RepeatedRow[T]] {
	if readSize <= 0 {
		panic("readSize must be greater than 0")
	}
	buffer := make([]parquet.Value, readSize)
	return &repeatedPageIterator[T]{
		rows:           rows,
		rgs:            rgs,
		column:         column,
		readSize:       readSize,
		buffer:         buffer,
		originalBuffer: buffer,
	}
}

func (it *repeatedPageIterator[T]) seekRowNum() int64 {
	return any(it.rows.At()).(RowGetter).RowNumber()
}

func (it *repeatedPageIterator[T]) Next() bool {
	// we should only next the first time and if we have reached a new row in the page.
	if !it.rows.Next() { // 1 [1 2] // 2
		return false
	}
	return it.next()
}

func (it *repeatedPageIterator[T]) next() bool {
	for {
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
			if err := it.currentPages.SeekToRow(it.seekRowNum() - it.startRowGroupRowNum); err != nil {
				it.err = err
				it.currentPages = nil
				return false
			}
			it.startPageRowNum = it.seekRowNum()
			var err error
			it.currentPage, err = it.currentPages.ReadPage()
			if err != nil {
				if err == io.EOF {
					continue
				}
				it.err = err
				return false
			}
			it.valueReader = nil
		}
		// if there's no more value in that page we can skip it.
		if it.seekRowNum() >= it.startPageRowNum+it.currentPage.NumRows() {
			it.currentPage = nil
			continue
		}

		if it.valueReader == nil {
			it.valueReader = it.currentPage.Values()
		}
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
			it.currentPage = nil
			continue
		}
		// slice the current amount of values.
		// todo start with 1 it's still the current row.
		// only one 0 => we don't increment the next rowNum
		// two zero => we increment the next rowNum

		//
		it.currentValue.Row = it.rows.At()

		next := 1
		for _, v := range it.buffer[1:] {
			if v.RepetitionLevel() == 0 {
				break
			}
			next++
		}
		it.currentValue.Values = it.buffer[:next]
		it.buffer = it.buffer[next:]
		if len(it.buffer) == 0 {
			it.valueReader = nil
		}
		return true
	}
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

func (it *repeatedPageIterator[T]) At() RepeatedRow[T] {
	return it.currentValue
}

func (it *repeatedPageIterator[T]) Err() error {
	return it.err
}

func (it *repeatedPageIterator[T]) Close() error {
	if it.currentPages != nil {
		if err := it.currentPages.Close(); err != nil {
			return err
		}
		it.currentPages = nil
	}
	return nil
}

/// => 2 4 [ 2 3 4] skip rows that I don't select
// => 2  10 {[ 2 3 4 5 6 7 8 9 ] } // skip through values and skip all values at once if possible.
// 2 5 6 15 {[1 2] [5 6] [10 11]} {[15]} skip through rows.
// 2 10  {[1 2] [5 6] [10 11]} {[15]} // skip through pages
