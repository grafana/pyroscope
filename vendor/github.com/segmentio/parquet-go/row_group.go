package parquet

import (
	"fmt"
	"io"
)

// RowGroup is an interface representing a parquet row group. From the Parquet
// docs, a RowGroup is "a logical horizontal partitioning of the data into rows.
// There is no physical structure that is guaranteed for a row group. A row
// group consists of a column chunk for each column in the dataset."
//
// https://github.com/apache/parquet-format#glossary
type RowGroup interface {
	// Returns the number of rows in the group.
	NumRows() int64

	// Returns the list of column chunks in this row group. The chunks are
	// ordered in the order of leaf columns from the row group's schema.
	//
	// If the underlying implementation is not read-only, the returned
	// parquet.ColumnChunk may implement other interfaces: for example,
	// parquet.ColumnBuffer if the chunk is backed by an in-memory buffer,
	// or typed writer interfaces like parquet.Int32Writer depending on the
	// underlying type of values that can be written to the chunk.
	//
	// As an optimization, the row group may return the same slice across
	// multiple calls to this method. Applications should treat the returned
	// slice as read-only.
	ColumnChunks() []ColumnChunk

	// Returns the schema of rows in the group.
	Schema() *Schema

	// Returns the list of sorting columns describing how rows are sorted in the
	// group.
	//
	// The method will return an empty slice if the rows are not sorted.
	SortingColumns() []SortingColumn

	// Returns a reader exposing the rows of the row group.
	//
	// As an optimization, the returned parquet.Rows object may implement
	// parquet.RowWriterTo, and test the RowWriter it receives for an
	// implementation of the parquet.RowGroupWriter interface.
	//
	// This optimization mechanism is leveraged by the parquet.CopyRows function
	// to skip the generic row-by-row copy algorithm and delegate the copy logic
	// to the parquet.Rows object.
	Rows() Rows
}

// Rows is an interface implemented by row readers returned by calling the Rows
// method of RowGroup instances.
//
// Applications should call Close when they are done using a Rows instance in
// order to release the underlying resources held by the row sequence.
//
// After calling Close, all attempts to read more rows will return io.EOF.
type Rows interface {
	RowReaderWithSchema
	RowSeeker
	io.Closer
}

// RowGroupReader is an interface implemented by types that expose sequences of
// row groups to the application.
type RowGroupReader interface {
	ReadRowGroup() (RowGroup, error)
}

// RowGroupWriter is an interface implemented by types that allow the program
// to write row groups.
type RowGroupWriter interface {
	WriteRowGroup(RowGroup) (int64, error)
}

// SortingColumn represents a column by which a row group is sorted.
type SortingColumn interface {
	// Returns the path of the column in the row group schema, omitting the name
	// of the root node.
	Path() []string

	// Returns true if the column will sort values in descending order.
	Descending() bool

	// Returns true if the column will put null values at the beginning.
	NullsFirst() bool
}

// Ascending constructs a SortingColumn value which dictates to sort the column
// at the path given as argument in ascending order.
func Ascending(path ...string) SortingColumn { return ascending(path) }

// Descending constructs a SortingColumn value which dictates to sort the column
// at the path given as argument in descending order.
func Descending(path ...string) SortingColumn { return descending(path) }

// NullsFirst wraps the SortingColumn passed as argument so that it instructs
// the row group to place null values first in the column.
func NullsFirst(sortingColumn SortingColumn) SortingColumn { return nullsFirst{sortingColumn} }

type ascending []string

func (asc ascending) String() string   { return fmt.Sprintf("ascending(%s)", columnPath(asc)) }
func (asc ascending) Path() []string   { return asc }
func (asc ascending) Descending() bool { return false }
func (asc ascending) NullsFirst() bool { return false }

type descending []string

func (desc descending) String() string   { return fmt.Sprintf("descending(%s)", columnPath(desc)) }
func (desc descending) Path() []string   { return desc }
func (desc descending) Descending() bool { return true }
func (desc descending) NullsFirst() bool { return false }

type nullsFirst struct{ SortingColumn }

func (nf nullsFirst) String() string   { return fmt.Sprintf("nulls_first+%s", nf.SortingColumn) }
func (nf nullsFirst) NullsFirst() bool { return true }

func searchSortingColumn(sortingColumns []SortingColumn, path columnPath) int {
	// There are usually a few sorting columns in a row group, so the linear
	// scan is the fastest option and works whether the sorting column list
	// is sorted or not. Please revisit this decision if this code path ends
	// up being more costly than necessary.
	for i, sorting := range sortingColumns {
		if path.equal(sorting.Path()) {
			return i
		}
	}
	return len(sortingColumns)
}

func sortingColumnsHavePrefix(sortingColumns, prefix []SortingColumn) bool {
	if len(sortingColumns) < len(prefix) {
		return false
	}
	for i, sortingColumn := range prefix {
		if !sortingColumnsAreEqual(sortingColumns[i], sortingColumn) {
			return false
		}
	}
	return true
}

func sortingColumnsAreEqual(s1, s2 SortingColumn) bool {
	path1 := columnPath(s1.Path())
	path2 := columnPath(s2.Path())
	return path1.equal(path2) && s1.Descending() == s2.Descending() && s1.NullsFirst() == s2.NullsFirst()
}

// MergeRowGroups constructs a row group which is a merged view of rowGroups. If
// rowGroups are sorted and the passed options include sorting, the merged row
// group will also be sorted.
//
// The function validates the input to ensure that the merge operation is
// possible, ensuring that the schemas match or can be converted to an
// optionally configured target schema passed as argument in the option list.
//
// The sorting columns of each row group are also consulted to determine whether
// the output can be represented. If sorting columns are configured on the merge
// they must be a prefix of sorting columns of all row groups being merged.
func MergeRowGroups(rowGroups []RowGroup, options ...RowGroupOption) (RowGroup, error) {
	config, err := NewRowGroupConfig(options...)
	if err != nil {
		return nil, err
	}

	schema := config.Schema
	if len(rowGroups) == 0 {
		return newEmptyRowGroup(schema), nil
	}
	if schema == nil {
		schema = rowGroups[0].Schema()

		for _, rowGroup := range rowGroups[1:] {
			if !nodesAreEqual(schema, rowGroup.Schema()) {
				return nil, ErrRowGroupSchemaMismatch
			}
		}
	}

	mergedRowGroups := make([]RowGroup, len(rowGroups))
	copy(mergedRowGroups, rowGroups)

	for i, rowGroup := range mergedRowGroups {
		if rowGroupSchema := rowGroup.Schema(); !nodesAreEqual(schema, rowGroupSchema) {
			conv, err := Convert(schema, rowGroupSchema)
			if err != nil {
				return nil, fmt.Errorf("cannot merge row groups: %w", err)
			}
			mergedRowGroups[i] = ConvertRowGroup(rowGroup, conv)
		}
	}

	m := &mergedRowGroup{sorting: config.SortingColumns}
	m.init(schema, mergedRowGroups)

	if len(m.sorting) == 0 {
		// When the row group has no ordering, use a simpler version of the
		// merger which simply concatenates rows from each of the row groups.
		// This is preferable because it makes the output deterministic, the
		// heap merge may otherwise reorder rows across groups.
		return &m.multiRowGroup, nil
	}

	for _, rowGroup := range m.rowGroups {
		if !sortingColumnsHavePrefix(rowGroup.SortingColumns(), m.sorting) {
			return nil, ErrRowGroupSortingColumnsMismatch
		}
	}

	m.sortFuncs = make([]columnSortFunc, len(m.sorting))
	forEachLeafColumnOf(schema, func(leaf leafColumn) {
		if sortingIndex := searchSortingColumn(m.sorting, leaf.path); sortingIndex < len(m.sorting) {
			m.sortFuncs[sortingIndex] = columnSortFunc{
				columnIndex: leaf.columnIndex,
				compare: sortFuncOf(
					leaf.node.Type(),
					&SortConfig{
						MaxRepetitionLevel: int(leaf.maxRepetitionLevel),
						MaxDefinitionLevel: int(leaf.maxDefinitionLevel),
						Descending:         m.sorting[sortingIndex].Descending(),
						NullsFirst:         m.sorting[sortingIndex].NullsFirst(),
					},
				),
			}
		}
	})

	return m, nil
}

type rowGroup struct {
	schema  *Schema
	numRows int64
	columns []ColumnChunk
	sorting []SortingColumn
}

func (r *rowGroup) NumRows() int64                  { return r.numRows }
func (r *rowGroup) ColumnChunks() []ColumnChunk     { return r.columns }
func (r *rowGroup) SortingColumns() []SortingColumn { return r.sorting }
func (r *rowGroup) Schema() *Schema                 { return r.schema }
func (r *rowGroup) Rows() Rows                      { return &rowGroupRows{rowGroup: r} }

func NewRowGroupRowReader(rowGroup RowGroup) Rows {
	return &rowGroupRows{rowGroup: rowGroup}
}

type rowGroupRows struct {
	rowGroup RowGroup
	columns  []columnChunkReader
	seek     int64
	inited   bool
	closed   bool
}

func (r *rowGroupRows) init() {
	const columnBufferSize = defaultValueBufferSize
	columns := r.rowGroup.ColumnChunks()
	buffer := make([]Value, columnBufferSize*len(columns))
	r.columns = make([]columnChunkReader, len(columns))

	for i, column := range columns {
		r.columns[i].buffer = buffer[:0:columnBufferSize]
		r.columns[i].reader = column.Pages()
		buffer = buffer[columnBufferSize:]
	}

	r.inited = true
}

func (r *rowGroupRows) Reset() {
	for i := range r.columns {
		// Ignore errors because we are resetting the reader, if the error
		// persists we will see it on the next read, and otherwise we can
		// read back from the beginning.
		r.columns[i].seekToRow(0)
	}
	r.seek = 0
}

func (r *rowGroupRows) Close() error {
	var lastErr error

	for i := range r.columns {
		if err := r.columns[i].close(); err != nil {
			lastErr = err
		}
	}

	r.inited = true
	r.closed = true
	return lastErr
}

func (r *rowGroupRows) Schema() *Schema {
	return r.rowGroup.Schema()
}

func (r *rowGroupRows) SeekToRow(rowIndex int64) error {
	if r.closed {
		return io.ErrClosedPipe
	}

	for i := range r.columns {
		if err := r.columns[i].seekToRow(rowIndex); err != nil {
			return err
		}
	}

	r.seek = rowIndex
	return nil
}

func (r *rowGroupRows) ReadRows(rows []Row) (int, error) {
	if !r.inited {
		r.init()
		if r.seek > 0 {
			if err := r.SeekToRow(r.seek); err != nil {
				return 0, err
			}
		}
	}

	if r.closed {
		return 0, io.EOF
	}

	for i := range rows {
		rows[i] = rows[i][:0]
	}

	return r.rowGroup.Schema().readRows(rows, 0, r.columns)
}

/*
func (r *rowGroupRows) WriteRowsTo(w RowWriter) (int64, error) {
	if r.rowGroup == nil {
		return CopyRows(w, struct{ RowReaderWithSchema }{r})
	}
	defer func() { r.rowGroup, r.seek = nil, 0 }()
	rowGroup := r.rowGroup
	if r.seek > 0 {
		columns := rowGroup.ColumnChunks()
		seekRowGroup := &seekRowGroup{
			base:    rowGroup,
			seek:    r.seek,
			columns: make([]ColumnChunk, len(columns)),
		}
		seekColumnChunks := make([]seekColumnChunk, len(columns))
		for i := range seekColumnChunks {
			seekColumnChunks[i].base = columns[i]
			seekColumnChunks[i].seek = r.seek
			seekRowGroup.columns[i] = &seekColumnChunks[i]
		}
		rowGroup = seekRowGroup
	}

	switch dst := w.(type) {
	case RowGroupWriter:
		return dst.WriteRowGroup(rowGroup)

	case PageWriter:
		for _, column := range rowGroup.ColumnChunks() {
			_, err := copyPagesAndClose(dst, column.Pages())
			if err != nil {
				return 0, err
			}
		}
		return rowGroup.NumRows(), nil
	}

	return CopyRows(w, struct{ RowReaderWithSchema }{r})
}

func (r *rowGroupRows) writeRowsTo(w pageAndValueWriter, limit int64) (numRows int64, err error) {
	for i := range r.columns {
		n, err := r.columns[i].writeRowsTo(w, limit)
		if err != nil {
			return numRows, err
		}
		if i == 0 {
			numRows = n
		} else if numRows != n {
			return numRows, fmt.Errorf("column %d wrote %d rows but the previous column(s) wrote %d rows", i, n, numRows)
		}
	}
	return numRows, nil
}
*/

type seekRowGroup struct {
	base    RowGroup
	seek    int64
	columns []ColumnChunk
}

func (g *seekRowGroup) NumRows() int64 {
	return g.base.NumRows() - g.seek
}

func (g *seekRowGroup) ColumnChunks() []ColumnChunk {
	return g.columns
}

func (g *seekRowGroup) Schema() *Schema {
	return g.base.Schema()
}

func (g *seekRowGroup) SortingColumns() []SortingColumn {
	return g.base.SortingColumns()
}

func (g *seekRowGroup) Rows() Rows {
	rows := g.base.Rows()
	rows.SeekToRow(g.seek)
	return rows
}

type seekColumnChunk struct {
	base ColumnChunk
	seek int64
}

func (c *seekColumnChunk) Type() Type {
	return c.base.Type()
}

func (c *seekColumnChunk) Column() int {
	return c.base.Column()
}

func (c *seekColumnChunk) Pages() Pages {
	pages := c.base.Pages()
	pages.SeekToRow(c.seek)
	return pages
}

func (c *seekColumnChunk) ColumnIndex() ColumnIndex {
	return c.base.ColumnIndex()
}

func (c *seekColumnChunk) OffsetIndex() OffsetIndex {
	return c.base.OffsetIndex()
}

func (c *seekColumnChunk) BloomFilter() BloomFilter {
	return c.base.BloomFilter()
}

func (c *seekColumnChunk) NumValues() int64 {
	return c.base.NumValues()
}

type emptyRowGroup struct {
	schema  *Schema
	columns []ColumnChunk
}

func newEmptyRowGroup(schema *Schema) *emptyRowGroup {
	columns := schema.Columns()
	rowGroup := &emptyRowGroup{
		schema:  schema,
		columns: make([]ColumnChunk, len(columns)),
	}
	emptyColumnChunks := make([]emptyColumnChunk, len(columns))
	for i, column := range schema.Columns() {
		leaf, _ := schema.Lookup(column...)
		emptyColumnChunks[i].typ = leaf.Node.Type()
		emptyColumnChunks[i].column = int16(leaf.ColumnIndex)
		rowGroup.columns[i] = &emptyColumnChunks[i]
	}
	return rowGroup
}

func (g *emptyRowGroup) NumRows() int64                  { return 0 }
func (g *emptyRowGroup) ColumnChunks() []ColumnChunk     { return g.columns }
func (g *emptyRowGroup) Schema() *Schema                 { return g.schema }
func (g *emptyRowGroup) SortingColumns() []SortingColumn { return nil }
func (g *emptyRowGroup) Rows() Rows                      { return emptyRows{g.schema} }

type emptyColumnChunk struct {
	typ    Type
	column int16
}

func (c *emptyColumnChunk) Type() Type               { return c.typ }
func (c *emptyColumnChunk) Column() int              { return int(c.column) }
func (c *emptyColumnChunk) Pages() Pages             { return emptyPages{} }
func (c *emptyColumnChunk) ColumnIndex() ColumnIndex { return emptyColumnIndex{} }
func (c *emptyColumnChunk) OffsetIndex() OffsetIndex { return emptyOffsetIndex{} }
func (c *emptyColumnChunk) BloomFilter() BloomFilter { return emptyBloomFilter{} }
func (c *emptyColumnChunk) NumValues() int64         { return 0 }

type emptyBloomFilter struct{}

func (emptyBloomFilter) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
func (emptyBloomFilter) Size() int64                       { return 0 }
func (emptyBloomFilter) Check(Value) (bool, error)         { return false, nil }

type emptyRows struct{ schema *Schema }

func (r emptyRows) Close() error                         { return nil }
func (r emptyRows) Schema() *Schema                      { return r.schema }
func (r emptyRows) ReadRows([]Row) (int, error)          { return 0, io.EOF }
func (r emptyRows) SeekToRow(int64) error                { return nil }
func (r emptyRows) WriteRowsTo(RowWriter) (int64, error) { return 0, nil }

type emptyPages struct{}

func (emptyPages) ReadPage() (Page, error) { return nil, io.EOF }
func (emptyPages) SeekToRow(int64) error   { return nil }
func (emptyPages) Close() error            { return nil }

var (
	_ RowReaderWithSchema = (*rowGroupRows)(nil)
	//_ RowWriterTo         = (*rowGroupRows)(nil)

	_ RowReaderWithSchema = emptyRows{}
	_ RowWriterTo         = emptyRows{}
)
