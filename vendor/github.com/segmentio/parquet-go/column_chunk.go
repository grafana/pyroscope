package parquet

import (
	"errors"
	"io"
)

// The ColumnChunk interface represents individual columns of a row group.
type ColumnChunk interface {
	// Returns the column type.
	Type() Type

	// Returns the index of this column in its parent row group.
	Column() int

	// Returns a reader exposing the pages of the column.
	Pages() Pages

	// Returns the components of the page index for this column chunk,
	// containing details about the content and location of pages within the
	// chunk.
	//
	// Note that the returned value may be the same across calls to these
	// methods, programs must treat those as read-only.
	//
	// If the column chunk does not have a page index, the methods return nil.
	ColumnIndex() ColumnIndex
	OffsetIndex() OffsetIndex
	BloomFilter() BloomFilter

	// Returns the number of values in the column chunk.
	//
	// This quantity may differ from the number of rows in the parent row group
	// because repeated columns may hold zero or more values per row.
	NumValues() int64
}

type pageAndValueWriter interface {
	PageWriter
	ValueWriter
}

type columnChunkReader struct {
	// These two fields must be configured to initialize the reader.
	buffer []Value     // buffer holding values read from the pages
	offset int         // offset of the next value in the buffer
	reader Pages       // reader of column pages
	values ValueReader // reader for values from the current page
}

func (r *columnChunkReader) buffered() int {
	return len(r.buffer) - r.offset
}

func (r *columnChunkReader) reset() {
	clearValues(r.buffer)
	r.buffer = r.buffer[:0]
	r.offset = 0
	r.values = nil
}

func (r *columnChunkReader) close() (err error) {
	r.reset()
	return r.reader.Close()
}

func (r *columnChunkReader) seekToRow(rowIndex int64) error {
	// TODO: there are a few optimizations we can make here:
	// * is the row buffered already? => advance the offset
	// * is the row in the current page? => seek in values
	r.reset()
	return r.reader.SeekToRow(rowIndex)
}

func (r *columnChunkReader) readValues() error {
	if r.offset < len(r.buffer) {
		return nil
	}
	if r.values == nil {
		for {
			p, err := r.reader.ReadPage()
			if err != nil {
				return err
			}
			if p.NumValues() > 0 {
				r.values = p.Values()
				break
			}
		}
	}
	n, err := r.values.ReadValues(r.buffer[:cap(r.buffer)])
	if errors.Is(err, io.EOF) {
		r.values = nil
	}
	if n > 0 {
		err = nil
	}
	r.buffer = r.buffer[:n]
	r.offset = 0
	return err
}

/*
func (r *columnChunkReader) writeBufferedRowsTo(w pageAndValueWriter, rowCount int64) (numRows int64, err error) {
	if rowCount == 0 {
		return 0, nil
	}

	for {
		for r.offset < len(r.buffer) {
			values := r.buffer[r.offset:]
			// We can only determine that the full row has been consumed if we
			// have more values in the buffer, and the next value is the start
			// of a new row. Otherwise, we have to load more values from the
			// page, which may yield EOF if all values have been consumed, in
			// which case we know that we have read the full row, and otherwise
			// we will enter this check again on the next loop iteration.
			if numRows == rowCount {
				if values[0].repetitionLevel == 0 {
					return numRows, nil
				}
				values, _ = splitRowValues(values)
			} else {
				values = limitRowValues(values, int(rowCount-numRows))
			}

			n, err := w.WriteValues(values)
			numRows += int64(countRowsOf(values[:n]))
			r.offset += n
			if err != nil {
				return numRows, err
			}
		}

		if err := r.readValuesFromCurrentPage(); err != nil {
			if err == io.EOF {
				err = nil
			}
			return numRows, err
		}
	}
}

func (r *columnChunkReader) writeRowsTo(w pageAndValueWriter, limit int64) (numRows int64, err error) {
	for numRows < limit {
		if r.values != nil {
			n, err := r.writeBufferedRowsTo(w, numRows-limit)
			numRows += n
			if err != nil || numRows == limit {
				return numRows, err
			}
		}

		r.buffer = r.buffer[:0]
		r.offset = 0

		for numRows < limit {
			p, err := r.reader.ReadPage()
			if err != nil {
				return numRows, err
			}

			pageRows := int64(p.NumRows())
			// When the page is fully contained in the remaining range of rows
			// that we intend to copy, we can use an optimized page copy rather
			// than writing rows one at a time.
			//
			// Data pages v1 do not expose the number of rows available, which
			// means we cannot take the optimized page copy path in those cases.
			if pageRows == 0 || int64(pageRows) > limit {
				r.values = p.Values()
				err := r.readValuesFromCurrentPage()
				if err == nil {
					// More values have been buffered, break out of the inner loop
					// to go back to the beginning of the outer loop and write
					// buffered values to the output.
					break
				}
				if errors.Is(err, io.EOF) {
					// The page contained no values? Unclear if this is valid but
					// we can handle it by reading the next page.
					r.values = nil
					continue
				}
				return numRows, err
			}

			if _, err := w.WritePage(p); err != nil {
				return numRows, err
			}

			numRows += pageRows
		}
	}
	return numRows, nil
}
*/

type readRowsFunc func([]Row, byte, []columnChunkReader) (int, error)

func readRowsFuncOf(node Node, columnIndex int, repetitionDepth byte) (int, readRowsFunc) {
	var read readRowsFunc

	if node.Repeated() {
		repetitionDepth++
	}

	if node.Leaf() {
		columnIndex, read = readRowsFuncOfLeaf(columnIndex, repetitionDepth)
	} else {
		columnIndex, read = readRowsFuncOfGroup(node, columnIndex, repetitionDepth)
	}

	if node.Repeated() {
		read = readRowsFuncOfRepeated(read, repetitionDepth)
	}

	return columnIndex, read
}

//go:noinline
func readRowsFuncOfRepeated(read readRowsFunc, repetitionDepth byte) readRowsFunc {
	return func(rows []Row, repetitionLevel byte, columns []columnChunkReader) (int, error) {
		for i := range rows {
			// Repeated columns have variable number of values, we must process
			// them one row at a time because we cannot predict how many values
			// need to be consumed in each iteration.
			row := rows[i : i+1]

			// The first pass looks for values marking the beginning of a row by
			// having a repetition level equal to the current level.
			n, err := read(row, repetitionLevel, columns)
			if err != nil {
				// The error here may likely be io.EOF, the read function may
				// also have successfully read a row, which is indicated by a
				// non-zero count. In this case, we increment the index to
				// indicate to the caller than rows up to i+1 have been read.
				if n > 0 {
					i++
				}
				return i, err
			}

			// The read function may return no errors and also read no rows in
			// case where it had more values to read but none corresponded to
			// the current repetition level. This is an indication that we will
			// not be able to read more rows at this stage, we must return to
			// the caller to let it set the repetition level to its current
			// depth, which may allow us to read more values when called again.
			if n == 0 {
				return i, nil
			}

			// When we reach this stage, we have successfully read the first
			// values of a row of repeated columns. We continue consuming more
			// repeated values until we get the indication that we consumed
			// them all (the read function returns zero and no errors).
			for {
				n, err := read(row, repetitionDepth, columns)
				if err != nil {
					return i + 1, err
				}
				if n == 0 {
					break
				}
			}
		}
		return len(rows), nil
	}
}

//go:noinline
func readRowsFuncOfGroup(node Node, columnIndex int, repetitionDepth byte) (int, readRowsFunc) {
	fields := node.Fields()

	if len(fields) == 0 {
		return columnIndex, func(_ []Row, _ byte, _ []columnChunkReader) (int, error) {
			return 0, io.EOF
		}
	}

	if len(fields) == 1 {
		// Small optimization for a somewhat common case of groups with a single
		// column (like nested list elements for example); there is no need to
		// loop over the group of a single element, we can simply skip to calling
		// the inner read function.
		return readRowsFuncOf(fields[0], columnIndex, repetitionDepth)
	}

	group := make([]readRowsFunc, len(fields))
	for i := range group {
		columnIndex, group[i] = readRowsFuncOf(fields[i], columnIndex, repetitionDepth)
	}

	return columnIndex, func(rows []Row, repetitionLevel byte, columns []columnChunkReader) (int, error) {
		// When reading a group, we use the first column as an indicator of how
		// may rows can be read during this call.
		n, err := group[0](rows, repetitionLevel, columns)

		if n > 0 {
			// Read values for all rows that the group is able to consume.
			// Getting io.EOF from calling the read functions indicate that
			// we consumed all values of that particular column, but there may
			// be more to read in other columns, therefore we must always read
			// all columns and cannot stop on the first error.
			for _, read := range group[1:] {
				_, err2 := read(rows[:n], repetitionLevel, columns)
				if err2 != nil && !errors.Is(err2, io.EOF) {
					return 0, err2
				}
			}
		}

		return n, err
	}
}

//go:noinline
func readRowsFuncOfLeaf(columnIndex int, repetitionDepth byte) (int, readRowsFunc) {
	var read readRowsFunc

	if repetitionDepth == 0 {
		read = func(rows []Row, _ byte, columns []columnChunkReader) (int, error) {
			// When the repetition depth is zero, we know that there is exactly
			// one value per row for this column, and therefore we can consume
			// as many values as there are rows to fill.
			col := &columns[columnIndex]

			for n := 0; n < len(rows); {
				if col.offset < len(col.buffer) {
					rows[n] = append(rows[n], col.buffer[col.offset])
					n++
					col.offset++
					continue
				}
				if err := col.readValues(); err != nil {
					return n, err
				}
			}

			return len(rows), nil
		}
	} else {
		read = func(rows []Row, repetitionLevel byte, columns []columnChunkReader) (int, error) {
			// When the repetition depth is not zero, we know that we will be
			// called with a single row as input. We attempt to read at most one
			// value of a single row and return to the caller.
			col := &columns[columnIndex]

			for {
				if col.offset < len(col.buffer) {
					if col.buffer[col.offset].repetitionLevel != repetitionLevel {
						return 0, nil
					}
					rows[0] = append(rows[0], col.buffer[col.offset])
					col.offset++
					return 1, nil
				}
				if err := col.readValues(); err != nil {
					return 0, err
				}
			}
		}
	}

	return columnIndex + 1, read
}
