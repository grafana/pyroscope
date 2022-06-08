package parquet

import (
	"fmt"
	"io"
	"sync"
)

// ConvertError is an error type returned by calls to Convert when the conversion
// of parquet schemas is impossible or the input row for the conversion is
// malformed.
type ConvertError struct {
	Path []string
	From Node
	To   Node
}

// Error satisfies the error interface.
func (e *ConvertError) Error() string {
	sourceType := e.From.Type()
	targetType := e.To.Type()

	sourceRepetition := fieldRepetitionTypeOf(e.From)
	targetRepetition := fieldRepetitionTypeOf(e.To)

	return fmt.Sprintf("cannot convert parquet column %q from %s %s to %s %s",
		columnPath(e.Path),
		sourceRepetition,
		sourceType,
		targetRepetition,
		targetType,
	)
}

// Conversion is an interface implemented by types that provide conversion of
// parquet rows from one schema to another.
//
// Conversion instances must be safe to use concurrently from multiple goroutines.
type Conversion interface {
	// Applies the conversion logic on the src row, returning the result
	// appended to dst.
	Convert(dst, src Row) (Row, error)
	// Converts the given column index in the target schema to the original
	// column index in the source schema of the conversion.
	Column(int) int
	// Returns the target schema of the conversion.
	Schema() *Schema
}

type conversion struct {
	targetColumnKinds   []Kind
	targetToSourceIndex []int16
	sourceToTargetIndex []int16
	schema              *Schema
	buffers             sync.Pool
}

type conversionBuffer struct {
	columns [][]Value
}

func (c *conversion) getBuffer() *conversionBuffer {
	b, _ := c.buffers.Get().(*conversionBuffer)
	if b == nil {
		n := len(c.targetColumnKinds)
		columns, values := make([][]Value, n), make([]Value, n)
		for i := range columns {
			columns[i] = values[i : i : i+1]
		}
		b = &conversionBuffer{columns: columns}
	}
	return b
}

func (c *conversion) putBuffer(b *conversionBuffer) {
	for i, values := range b.columns {
		clearValues(values)
		b.columns[i] = values[:0]
	}
	c.buffers.Put(b)
}

func (c *conversion) Convert(target, source Row) (Row, error) {
	buffer := c.getBuffer()
	defer c.putBuffer(buffer)

	for _, value := range source {
		sourceIndex := value.Column()
		targetIndex := c.sourceToTargetIndex[sourceIndex]
		if targetIndex >= 0 {
			value.kind = ^int8(c.targetColumnKinds[targetIndex])
			value.columnIndex = ^targetIndex
			buffer.columns[targetIndex] = append(buffer.columns[targetIndex], value)
		}
	}

	for i, values := range buffer.columns {
		if len(values) == 0 {
			values = append(values, Value{
				kind:        ^int8(c.targetColumnKinds[i]),
				columnIndex: ^int16(i),
			})
		}
		target = append(target, values...)
	}

	return target, nil
}

func (c *conversion) Column(i int) int {
	return int(c.targetToSourceIndex[i])
}

func (c *conversion) Schema() *Schema {
	return c.schema
}

type identity struct{ schema *Schema }

func (id identity) Convert(dst, src Row) (Row, error) { return append(dst, src...), nil }
func (id identity) Column(i int) int                  { return i }
func (id identity) Schema() *Schema                   { return id.schema }

// Convert constructs a conversion function from one parquet schema to another.
//
// The function supports converting between schemas where the source or target
// have extra columns; if there are more columns in the source, they will be
// stripped out of the rows. Extra columns in the target schema will be set to
// null or zero values.
//
// The returned function is intended to be used to append the converted source
// row to the destination buffer.
func Convert(to, from Node) (conv Conversion, err error) {
	schema, _ := to.(*Schema)
	if schema == nil {
		schema = NewSchema("", to)
	}

	if nodesAreEqual(to, from) {
		return identity{schema}, nil
	}

	targetMapping, targetColumns := columnMappingOf(to)
	sourceMapping, sourceColumns := columnMappingOf(from)

	columnIndexBuffer := make([]int16, len(targetColumns)+len(sourceColumns))
	targetColumnKinds := make([]Kind, len(targetColumns))
	targetToSourceIndex := columnIndexBuffer[:len(targetColumns)]
	sourceToTargetIndex := columnIndexBuffer[len(targetColumns):]

	for i, path := range targetColumns {
		sourceColumn := sourceMapping.lookup(path)
		targetColumn := targetMapping.lookup(path)
		targetToSourceIndex[i] = sourceColumn.columnIndex
		targetColumnKinds[i] = targetColumn.node.Type().Kind()
	}

	for i, path := range sourceColumns {
		sourceColumn := sourceMapping.lookup(path)
		targetColumn := targetMapping.lookup(path)

		if targetColumn.node != nil {
			sourceType := sourceColumn.node.Type()
			targetType := targetColumn.node.Type()
			if sourceType.Kind() != targetType.Kind() {
				return nil, &ConvertError{Path: path, From: sourceColumn.node, To: targetColumn.node}
			}

			sourceRepetition := fieldRepetitionTypeOf(sourceColumn.node)
			targetRepetition := fieldRepetitionTypeOf(targetColumn.node)
			if sourceRepetition != targetRepetition {
				return nil, &ConvertError{Path: path, From: sourceColumn.node, To: targetColumn.node}
			}
		}

		sourceToTargetIndex[i] = targetColumn.columnIndex
	}

	return &conversion{
		targetColumnKinds:   targetColumnKinds,
		targetToSourceIndex: targetToSourceIndex,
		sourceToTargetIndex: sourceToTargetIndex,
		schema:              schema,
	}, nil
}

// ConvertRowGroup constructs a wrapper of the given row group which applies
// the given schema conversion to its rows.
func ConvertRowGroup(rowGroup RowGroup, conv Conversion) RowGroup {
	schema := conv.Schema()
	numRows := rowGroup.NumRows()
	rowGroupColumns := rowGroup.ColumnChunks()

	columns := make([]ColumnChunk, numLeafColumnsOf(schema))
	forEachLeafColumnOf(schema, func(leaf leafColumn) {
		i := leaf.columnIndex
		j := conv.Column(int(leaf.columnIndex))
		if j < 0 {
			columns[i] = &missingColumnChunk{
				typ:    leaf.node.Type(),
				column: i,
				// TODO: we assume the number of values is the same as the
				// number of rows, which may not be accurate when the column is
				// part of a repeated group; neighbor columns may be repeated in
				// which case it would be impossible for this chunk not to be.
				numRows:   numRows,
				numValues: numRows,
				numNulls:  numRows,
			}
		} else {
			columns[i] = rowGroupColumns[j]
		}
	})

	// Sorting columns must exist on the conversion schema in order to be
	// advertised on the converted row group otherwise the resulting rows
	// would not be in the right order.
	sorting := []SortingColumn{}
	for _, col := range rowGroup.SortingColumns() {
		if !hasColumnPath(schema, col.Path()) {
			break
		}
		sorting = append(sorting, col)
	}

	return &convertedRowGroup{
		// The pair of rowGroup+conv is retained to construct a converted row
		// reader by wrapping the underlying row reader of the row group because
		// it allows proper reconstruction of the repetition and definition
		// levels.
		//
		// TODO: can we figure out how to set the repetition and definition
		// levels when reading values from missing column pages? At first sight
		// it appears complex to do, however:
		//
		// * It is possible that having these levels when reading values of
		//   missing column pages is not necessary in some scenarios (e.g. when
		//   merging row groups).
		//
		// * We may be able to assume the repetition and definition levels at
		//   the call site (e.g. in the functions reading rows from columns).
		//
		// Columns of the source row group which do not exist in the target are
		// masked to prevent loading unneeded pages when reading rows from the
		// converted row group.
		rowGroup: maskMissingRowGroupColumns(rowGroup, len(columns), conv),
		columns:  columns,
		sorting:  sorting,
		conv:     conv,
	}
}

func maskMissingRowGroupColumns(r RowGroup, numColumns int, conv Conversion) RowGroup {
	rowGroupColumns := r.ColumnChunks()
	columns := make([]ColumnChunk, len(rowGroupColumns))
	missing := make([]missingColumnChunk, len(columns))
	numRows := r.NumRows()

	for i := range missing {
		missing[i] = missingColumnChunk{
			typ:       rowGroupColumns[i].Type(),
			column:    int16(i),
			numRows:   numRows,
			numValues: numRows,
			numNulls:  numRows,
		}
	}

	for i := range columns {
		columns[i] = &missing[i]
	}

	for i := 0; i < numColumns; i++ {
		j := conv.Column(i)
		if j >= 0 && j < len(columns) {
			columns[j] = rowGroupColumns[j]
		}
	}

	return &rowGroup{
		schema:  r.Schema(),
		numRows: numRows,
		columns: columns,
	}
}

type missingColumnChunk struct {
	typ       Type
	column    int16
	numRows   int64
	numValues int64
	numNulls  int64
}

func (c *missingColumnChunk) Type() Type               { return c.typ }
func (c *missingColumnChunk) Column() int              { return int(c.column) }
func (c *missingColumnChunk) Pages() Pages             { return onePage(missingPage{c}) }
func (c *missingColumnChunk) ColumnIndex() ColumnIndex { return missingColumnIndex{c} }
func (c *missingColumnChunk) OffsetIndex() OffsetIndex { return missingOffsetIndex{} }
func (c *missingColumnChunk) BloomFilter() BloomFilter { return missingBloomFilter{} }
func (c *missingColumnChunk) NumValues() int64         { return 0 }

type missingColumnIndex struct{ *missingColumnChunk }

func (i missingColumnIndex) NumPages() int       { return 1 }
func (i missingColumnIndex) NullCount(int) int64 { return i.numNulls }
func (i missingColumnIndex) NullPage(int) bool   { return true }
func (i missingColumnIndex) MinValue(int) Value  { return Value{} }
func (i missingColumnIndex) MaxValue(int) Value  { return Value{} }
func (i missingColumnIndex) IsAscending() bool   { return true }
func (i missingColumnIndex) IsDescending() bool  { return false }

type missingOffsetIndex struct{}

func (missingOffsetIndex) NumPages() int                { return 1 }
func (missingOffsetIndex) Offset(int) int64             { return 0 }
func (missingOffsetIndex) CompressedPageSize(int) int64 { return 0 }
func (missingOffsetIndex) FirstRowIndex(int) int64      { return 0 }

type missingBloomFilter struct{}

func (missingBloomFilter) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
func (missingBloomFilter) Size() int64                       { return 0 }
func (missingBloomFilter) Check(Value) (bool, error)         { return false, nil }

type missingPage struct{ *missingColumnChunk }

func (p missingPage) Column() int                       { return int(p.column) }
func (p missingPage) Dictionary() Dictionary            { return nil }
func (p missingPage) NumRows() int64                    { return p.numRows }
func (p missingPage) NumValues() int64                  { return p.numValues }
func (p missingPage) NumNulls() int64                   { return p.numNulls }
func (p missingPage) Bounds() (min, max Value, ok bool) { return }
func (p missingPage) Size() int64                       { return 0 }
func (p missingPage) Values() ValueReader               { return &missingPageValues{page: p} }
func (p missingPage) Buffer() BufferedPage {
	return newErrorPage(p.Type(), p.Column(), "cannot buffer missing page")
}

type missingPageValues struct {
	page missingPage
	read int64
}

func (r *missingPageValues) ReadValues(values []Value) (int, error) {
	remain := r.page.numValues - r.read
	if int64(len(values)) > remain {
		values = values[:remain]
	}
	for i := range values {
		// TODO: how do we set the repetition and definition levels here?
		values[i] = Value{columnIndex: ^r.page.column}
	}
	if r.read += int64(len(values)); r.read == r.page.numValues {
		return len(values), io.EOF
	}
	return len(values), nil
}

func (r *missingPageValues) Close() error {
	r.read = r.page.numValues
	return nil
}

type convertedRowGroup struct {
	rowGroup RowGroup
	columns  []ColumnChunk
	sorting  []SortingColumn
	conv     Conversion
}

func (c *convertedRowGroup) NumRows() int64                  { return c.rowGroup.NumRows() }
func (c *convertedRowGroup) ColumnChunks() []ColumnChunk     { return c.columns }
func (c *convertedRowGroup) Schema() *Schema                 { return c.conv.Schema() }
func (c *convertedRowGroup) SortingColumns() []SortingColumn { return c.sorting }
func (c *convertedRowGroup) Rows() Rows {
	rows := c.rowGroup.Rows()
	return &convertedRows{
		Closer: rows,
		rows:   rows,
		conv:   c.conv,
	}
}

// ConvertRowReader constructs a wrapper of the given row reader which applies
// the given schema conversion to the rows.
func ConvertRowReader(rows RowReader, conv Conversion) RowReaderWithSchema {
	return &convertedRows{rows: &forwardRowSeeker{rows: rows}, conv: conv}
}

type convertedRows struct {
	io.Closer
	rows RowReadSeeker
	buf  Row
	conv Conversion
}

func (c *convertedRows) ReadRows(rows []Row) (int, error) {
	maxRowLen := 0
	defer func() {
		clearValues(c.buf[:maxRowLen])
	}()

	n, err := c.rows.ReadRows(rows)

	for i, row := range rows[:n] {
		var err error
		c.buf, err = c.conv.Convert(c.buf[:0], row)
		if len(c.buf) > maxRowLen {
			maxRowLen = len(c.buf)
		}
		if err != nil {
			return i, err
		}
		rows[i] = append(row[:0], c.buf...)
	}

	return n, err
}

func (c *convertedRows) Schema() *Schema {
	return c.conv.Schema()
}

func (c *convertedRows) SeekToRow(rowIndex int64) error {
	return c.rows.SeekToRow(rowIndex)
}
