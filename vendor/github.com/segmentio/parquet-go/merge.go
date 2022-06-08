package parquet

import (
	"container/heap"
	"fmt"
	"io"
)

type mergedRowGroup struct {
	multiRowGroup
	sorting   []SortingColumn
	sortFuncs []columnSortFunc
}

func (m *mergedRowGroup) SortingColumns() []SortingColumn {
	return m.sorting
}

func (m *mergedRowGroup) Rows() Rows {
	// The row group needs to respect a sorting order; the merged row reader
	// uses a heap to merge rows from the row groups.
	return &mergedRowGroupRows{rowGroup: m, schema: m.schema}
}

type mergedRowGroupRows struct {
	rowGroup *mergedRowGroup
	schema   *Schema
	sorting  []columnSortFunc
	cursors  []rowGroupCursor
	values1  []Value
	values2  []Value
	seek     int64
	index    int64
	err      error
}

func (r *mergedRowGroupRows) init(m *mergedRowGroup) {
	if r.schema != nil {
		numColumns := numLeafColumnsOf(r.schema)
		cursors := make([]bufferedRowGroupCursor, len(m.rowGroups))
		buffers := make([][]Value, int(numColumns)*len(m.rowGroups))

		for i, rowGroup := range m.rowGroups {
			cursors[i].reader = rowGroup.Rows()
			cursors[i].columns, buffers = buffers[:numColumns:numColumns], buffers[numColumns:]
		}

		r.cursors = make([]rowGroupCursor, 0, len(cursors))
		r.sorting = m.sortFuncs

		for i := range cursors {
			c := rowGroupCursor(&cursors[i])
			// TODO: this is a bit of a weak model, it only works with types
			// declared in this package; we may want to define an API to allow
			// applications to participate in it.
			if rd, ok := cursors[i].reader.(*rowGroupRows); ok {
				rd.init()
				// TODO: this optimization is disabled for now, there are
				// remaining blockers:
				//
				// * The optimized merge of pages for non-overlapping ranges is
				//   not yet implemented in the mergedRowGroupRows type.
				//
				// * Using pages min/max to determine overlapping ranges does
				//   not work for repeated columns; sorting by repeated columns
				//   seems to be an edge case so probably not worth optimizing,
				//   we still need to find a way to disable the optimization in
				//   those cases.
				//
				//c = optimizedRowGroupCursor{rd}
			}

			if err := c.readNext(); err != nil {
				if err == io.EOF {
					continue
				}
				r.err = err
				return
			}

			r.cursors = append(r.cursors, c)
		}

		heap.Init(r)
	}
}

func (r *mergedRowGroupRows) SeekToRow(rowIndex int64) error {
	if rowIndex >= r.index {
		r.seek = rowIndex
		return nil
	}
	return fmt.Errorf("SeekToRow: merged row reader cannot seek backward from row %d to %d", r.index, rowIndex)
}

func (r *mergedRowGroupRows) ReadRows(rows []Row) (n int, err error) {
	if r.rowGroup != nil {
		r.init(r.rowGroup)
		r.rowGroup = nil
	}
	if r.err != nil {
		return 0, r.err
	}

	for n < len(rows) && len(r.cursors) > 0 {
		min := r.cursors[0]
		r.values1, err = min.readRow(r.values1[:0])
		if err != nil {
			return n, err
		}

		if r.index >= r.seek {
			rows[n] = append(rows[n][:0], r.values1...)
			n++
		}
		r.index++

		if err := min.readNext(); err != nil {
			if err != io.EOF {
				r.err = err
				return n, err
			}
			heap.Pop(r)
		} else {
			heap.Fix(r, 0)
		}
	}

	if n < len(rows) {
		err = io.EOF
	}

	return n, err
}

func (r *mergedRowGroupRows) Close() error {
	var lastErr error
	for i := range r.cursors {
		if err := r.cursors[i].close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// func (r *mergedRowGroupRows) WriteRowsTo(w RowWriter) (int64, error) {
// 	if r.rowGroup != nil {
// 		defer func() { r.rowGroup = nil }()
// 		switch dst := w.(type) {
// 		case RowGroupWriter:
// 			return dst.WriteRowGroup(r.rowGroup)
// 		case pageAndValueWriter:
// 			r.init(r.rowGroup)
// 			return r.writeRowsTo(dst)
// 		}
// 	}
// 	return CopyRows(w, struct{ RowReaderWithSchema }{r})
// }

func (r *mergedRowGroupRows) writeRowsTo(w pageAndValueWriter) (numRows int64, err error) {
	// TODO: the intent of this method is to optimize the merge of rows by
	// copying entire pages instead of individual rows when we detect ranges
	// that don't overlap.
	return
}

func (r *mergedRowGroupRows) Schema() *Schema {
	return r.schema
}

func (r *mergedRowGroupRows) Len() int {
	return len(r.cursors)
}

func (r *mergedRowGroupRows) Less(i, j int) bool {
	cursor1 := r.cursors[i]
	cursor2 := r.cursors[j]

	for _, sorting := range r.sorting {
		r.values1 = cursor1.nextRowValuesOf(r.values1[:0], sorting.columnIndex)
		r.values2 = cursor2.nextRowValuesOf(r.values2[:0], sorting.columnIndex)
		comp := sorting.compare(r.values1, r.values2)
		switch {
		case comp < 0:
			return true
		case comp > 0:
			return false
		}
	}

	return false
}

func (r *mergedRowGroupRows) Swap(i, j int) {
	r.cursors[i], r.cursors[j] = r.cursors[j], r.cursors[i]
}

func (r *mergedRowGroupRows) Push(interface{}) {
	panic("BUG: unreachable")
}

func (r *mergedRowGroupRows) Pop() interface{} {
	n := len(r.cursors) - 1
	c := r.cursors[n]
	r.cursors = r.cursors[:n]
	return c
}

type rowGroupCursor interface {
	close() error
	readRow(Row) (Row, error)
	readNext() error
	nextRowValuesOf([]Value, int16) []Value
}

type columnSortFunc struct {
	columnIndex int16
	compare     SortFunc
}

type bufferedRowGroupCursor struct {
	reader  Rows
	rowbuf  [1]Row
	columns [][]Value
}

func (cur *bufferedRowGroupCursor) close() error {
	return cur.reader.Close()
}

func (cur *bufferedRowGroupCursor) readRow(row Row) (Row, error) {
	return append(row, cur.rowbuf[0]...), nil
}

func (cur *bufferedRowGroupCursor) readNext() error {
	_, err := cur.reader.ReadRows(cur.rowbuf[:])
	if err != nil {
		return err
	}
	for i, c := range cur.columns {
		cur.columns[i] = c[:0]
	}
	for _, v := range cur.rowbuf[0] {
		columnIndex := v.Column()
		cur.columns[columnIndex] = append(cur.columns[columnIndex], v)
	}
	return nil
}

func (cur *bufferedRowGroupCursor) nextRowValuesOf(values []Value, columnIndex int16) []Value {
	return append(values, cur.columns[columnIndex]...)
}

/*
type optimizedRowGroupCursor struct{ *rowGroupRows }

func (cur optimizedRowGroupCursor) readRow(row Row) (Row, error) { return cur.ReadRow(row) }

func (cur optimizedRowGroupCursor) readNext() error {
	for i := range cur.columns {
		c := &cur.columns[i]
		if c.buffered() == 0 {
			if err := c.readPage(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cur optimizedRowGroupCursor) nextRowValuesOf(values []Value, columnIndex int16) []Value {
	col := &cur.columns[columnIndex]
	err := col.readValues()
	if err != nil {
		values = append(values, Value{})
	} else {
		values = append(values, col.buffer[col.offset])
	}
	return values
}
*/

var (
	_ RowReaderWithSchema = (*mergedRowGroupRows)(nil)
	//_ RowWriterTo         = (*mergedRowGroupRows)(nil)
)
