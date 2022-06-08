package parquet

import (
	"sort"
)

// Buffer represents an in-memory group of parquet rows.
//
// The main purpose of the Buffer type is to provide a way to sort rows before
// writing them to a parquet file. Buffer implements sort.Interface as a way
// to support reordering the rows that have been written to it.
type Buffer struct {
	config  *RowGroupConfig
	schema  *Schema
	rowbuf  []Row
	colbuf  [][]Value
	chunks  []ColumnChunk
	columns []ColumnBuffer
	sorted  []ColumnBuffer
}

// NewBuffer constructs a new buffer, using the given list of buffer options
// to configure the buffer returned by the function.
//
// The function panics if the buffer configuration is invalid. Programs that
// cannot guarantee the validity of the options passed to NewBuffer should
// construct the buffer configuration independently prior to calling this
// function:
//
//	config, err := parquet.NewRowGroupConfig(options...)
//	if err != nil {
//		// handle the configuration error
//		...
//	} else {
//		// this call to create a buffer is guaranteed not to panic
//		buffer := parquet.NewBuffer(config)
//		...
//	}
//
func NewBuffer(options ...RowGroupOption) *Buffer {
	config, err := NewRowGroupConfig(options...)
	if err != nil {
		panic(err)
	}
	buf := &Buffer{
		config: config,
	}
	if config.Schema != nil {
		buf.configure(config.Schema)
	}
	return buf
}

func (buf *Buffer) configure(schema *Schema) {
	if schema == nil {
		return
	}
	sortingColumns := buf.config.SortingColumns
	buf.sorted = make([]ColumnBuffer, len(sortingColumns))

	forEachLeafColumnOf(schema, func(leaf leafColumn) {
		nullOrdering := nullsGoLast
		columnIndex := int(leaf.columnIndex)
		columnType := leaf.node.Type()
		bufferSize := buf.config.ColumnBufferSize
		dictionary := (Dictionary)(nil)
		encoding := encodingOf(leaf.node)

		if isDictionaryEncoding(encoding) {
			bufferSize /= 2
			dictionary = columnType.NewDictionary(columnIndex, 0, make([]byte, 0, bufferSize))
			columnType = dictionary.Type()
		}

		column := columnType.NewColumnBuffer(columnIndex, bufferSize)
		switch {
		case leaf.maxRepetitionLevel > 0:
			column = newRepeatedColumnBuffer(column, leaf.maxRepetitionLevel, leaf.maxDefinitionLevel, nullOrdering)
		case leaf.maxDefinitionLevel > 0:
			column = newOptionalColumnBuffer(column, leaf.maxDefinitionLevel, nullOrdering)
		}
		buf.columns = append(buf.columns, column)

		if sortingIndex := searchSortingColumn(sortingColumns, leaf.path); sortingIndex < len(sortingColumns) {
			if sortingColumns[sortingIndex].Descending() {
				column = &reversedColumnBuffer{column}
			}
			if sortingColumns[sortingIndex].NullsFirst() {
				nullOrdering = nullsGoFirst
			}
			buf.sorted[sortingIndex] = column
		}
	})

	buf.schema = schema
	buf.rowbuf = make([]Row, 0, 1)
	buf.colbuf = make([][]Value, len(buf.columns))
	buf.chunks = make([]ColumnChunk, len(buf.columns))

	for i, column := range buf.columns {
		buf.chunks[i] = column
	}
}

// Size returns the estimated size of the buffer in memory (in bytes).
func (buf *Buffer) Size() int64 {
	size := int64(0)
	for _, col := range buf.columns {
		size += col.Size()
	}
	return size
}

// NumRows returns the number of rows written to the buffer.
func (buf *Buffer) NumRows() int64 { return int64(buf.Len()) }

// ColumnChunks returns the buffer columns.
func (buf *Buffer) ColumnChunks() []ColumnChunk { return buf.chunks }

// ColumnBuffer returns the buffer columns.
//
// This method is similar to ColumnChunks, but returns a list of ColumnBuffer
// instead of a ColumnChunk values (the latter being read-only); calling
// ColumnBuffers or ColumnChunks with the same index returns the same underlying
// objects, but with different types, which removes the need for making a type
// assertion if the program needed to write directly to the column buffers.
// The presence of the ColumnChunks method is still required to satisfy the
// RowGroup interface.
func (buf *Buffer) ColumnBuffers() []ColumnBuffer { return buf.columns }

// Schema returns the schema of the buffer.
//
// The schema is either configured by passing a Schema in the option list when
// constructing the buffer, or lazily discovered when the first row is written.
func (buf *Buffer) Schema() *Schema { return buf.schema }

// SortingColumns returns the list of columns by which the buffer will be
// sorted.
//
// The sorting order is configured by passing a SortingColumns option when
// constructing the buffer.
func (buf *Buffer) SortingColumns() []SortingColumn { return buf.config.SortingColumns }

// Len returns the number of rows written to the buffer.
func (buf *Buffer) Len() int {
	if len(buf.columns) == 0 {
		return 0
	} else {
		// All columns have the same number of rows.
		return buf.columns[0].Len()
	}
}

// Less returns true if row[i] < row[j] in the buffer.
func (buf *Buffer) Less(i, j int) bool {
	for _, col := range buf.sorted {
		switch {
		case col.Less(i, j):
			return true
		case col.Less(j, i):
			return false
		}
	}
	return false
}

// Swap exchanges the rows at indexes i and j.
func (buf *Buffer) Swap(i, j int) {
	for _, col := range buf.columns {
		col.Swap(i, j)
	}
}

// Reset clears the content of the buffer, allowing it to be reused.
func (buf *Buffer) Reset() {
	for _, col := range buf.columns {
		col.Reset()
	}
}

// Write writes a row held in a Go value to the buffer.
func (buf *Buffer) Write(row interface{}) error {
	if buf.schema == nil {
		buf.configure(SchemaOf(row))
	}

	buf.rowbuf = buf.rowbuf[:1]
	defer clearRows(buf.rowbuf)

	buf.rowbuf[0] = buf.schema.Deconstruct(buf.rowbuf[0], row)
	_, err := buf.WriteRows(buf.rowbuf)
	return err
}

// WriteRows writes parquet rows to the buffer.
func (buf *Buffer) WriteRows(rows []Row) (int, error) {
	defer func() {
		for i, colbuf := range buf.colbuf {
			clearValues(colbuf)
			buf.colbuf[i] = colbuf[:0]
		}
	}()

	if buf.schema == nil {
		return 0, ErrRowGroupSchemaMissing
	}

	for _, row := range rows {
		for _, value := range row {
			columnIndex := value.Column()
			buf.colbuf[columnIndex] = append(buf.colbuf[columnIndex], value)
		}
	}

	for columnIndex, values := range buf.colbuf {
		if _, err := buf.columns[columnIndex].WriteValues(values); err != nil {
			// TOOD: an error at this stage will leave the buffer in an invalid
			// state since the row was partially written. Applications are not
			// expected to continue using the buffer after getting an error,
			// maybe we can enforce it?
			return 0, err
		}
	}

	return len(rows), nil
}

// WriteRowGroup satisfies the RowGroupWriter interface.
func (buf *Buffer) WriteRowGroup(rowGroup RowGroup) (int64, error) {
	rowGroupSchema := rowGroup.Schema()
	switch {
	case rowGroupSchema == nil:
		return 0, ErrRowGroupSchemaMissing
	case buf.schema == nil:
		buf.configure(rowGroupSchema)
	case !nodesAreEqual(buf.schema, rowGroupSchema):
		return 0, ErrRowGroupSchemaMismatch
	}
	if !sortingColumnsHavePrefix(rowGroup.SortingColumns(), buf.SortingColumns()) {
		return 0, ErrRowGroupSortingColumnsMismatch
	}
	n := buf.NumRows()
	r := rowGroup.Rows()
	defer r.Close()
	_, err := CopyRows(bufferWriter{buf}, r)
	return buf.NumRows() - n, err
}

// Rows returns a reader exposing the current content of the buffer.
//
// The buffer and the returned reader share memory. Mutating the buffer
// concurrently to reading rows may result in non-deterministic behavior.
func (buf *Buffer) Rows() Rows { return &rowGroupRows{rowGroup: buf} }

// bufferWriter is an adapter for Buffer which implements both RowWriter and
// PageWriter to enable optimizations in CopyRows for types that support writing
// rows by copying whole pages instead of calling WriteRow repeatedly.
type bufferWriter struct{ buf *Buffer }

func (w bufferWriter) WriteRows(rows []Row) (int, error) {
	return w.buf.WriteRows(rows)
}

func (w bufferWriter) WriteValues(values []Value) (int, error) {
	return w.buf.columns[values[0].Column()].WriteValues(values)
}

func (w bufferWriter) WritePage(page Page) (int64, error) {
	return CopyValues(w.buf.columns[page.Column()], page.Values())
}

var (
	_ RowGroup       = (*Buffer)(nil)
	_ RowGroupWriter = (*Buffer)(nil)
	_ sort.Interface = (*Buffer)(nil)

	_ RowWriter   = (*bufferWriter)(nil)
	_ PageWriter  = (*bufferWriter)(nil)
	_ ValueWriter = (*bufferWriter)(nil)
)
