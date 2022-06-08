package dynparquet

import (
	"io"

	"github.com/segmentio/parquet-go"
)

// NilColumnChunk is a column chunk that contains a single page with all null
// values of the given type, given length and column index of the parent
// schema. It implements the parquet.ColumnChunk interface.
type NilColumnChunk struct {
	typ         parquet.Type
	columnIndex int
	numValues   int
}

// NewNilColumnChunk creates a new column chunk configured with the given type,
// column index and number of values in the page.
func NewNilColumnChunk(typ parquet.Type, columnIndex, numValues int) *NilColumnChunk {
	return &NilColumnChunk{
		typ:         typ,
		columnIndex: columnIndex,
		numValues:   numValues,
	}
}

// NumValues returns the number of values in the column chunk. Implements the
// parquet.ColumnChunk interface.
func (c *NilColumnChunk) NumValues() int64 {
	return int64(c.numValues)
}

// Type returns the type of the column chunk. Implements the
// parquet.ColumnChunk interface.
func (c *NilColumnChunk) Type() parquet.Type {
	return c.typ
}

// Type returns the index of the column chunk within the parent schema.
// Implements the parquet.ColumnChunk interface.
func (c *NilColumnChunk) Column() int {
	return c.columnIndex
}

// Pages returns an iterator for all pages within the column chunk. This
// iterator will only ever return a single page filled with all null values of
// the configured amount. Implements the parquet.ColumnChunk interface.
func (c *NilColumnChunk) Pages() parquet.Pages {
	return &nilPages{
		numValues:   c.numValues,
		columnIndex: c.columnIndex,
		typ:         c.typ,
	}
}

// nilPages is an iterator that will only ever return a single page filled with
// all null values of the configured amount. It knows the column index of the
// schema it belongs to. It implements the parquet.Pages interface.
type nilPages struct {
	numValues   int
	columnIndex int
	read        bool
	seek        int
	typ         parquet.Type
}

// ReadPage returns the next page in the column chunk. It will only ever return
// a single page which returns all null values of the configured amount.
// Implements the parquet.Pages interface.
func (p *nilPages) ReadPage() (parquet.Page, error) {
	if p.read {
		return nil, io.EOF
	}
	p.read = true

	return &nilPage{
		numValues:   p.numValues,
		columnIndex: p.columnIndex,
		seek:        p.seek,
		typ:         p.typ,
	}, nil
}

// Close implements the parquet.Pages interface. Since this is a synthetic
// page, it's a no-op.
func (p *nilPages) Close() error {
	return nil
}

// nilPage is a page that contains all null values of the configured amount. It
// is aware of the column index of the parent schema it belongs to. It
// implements the parquet.Page interface.
type nilPage struct {
	numValues   int
	columnIndex int
	seek        int
	typ         parquet.Type
}

// Column returns the column index of the column in the schema the column
// chunk's page belongs to.
func (p *nilPage) Column() int {
	return p.columnIndex
}

// Type returns the type of the column chunk. Implements the
// parquet.ColumnChunk interface.
func (p *nilPage) Type() parquet.Type {
	return p.typ
}

// Dictionary returns the dictionary page for the column chunk. Since the page
// only contains null values, the dictionary is always nil.
func (p *nilPage) Dictionary() parquet.Dictionary {
	// TODO: Validate that this doesn't require to an empty dictionary of the
	// correct type.
	return nil
}

// NumRows returns the number of rows the page contains.
func (p *nilPage) NumRows() int64 {
	return int64(p.numValues)
}

// NumValues returns the number of values the page contains.
func (p *nilPage) NumValues() int64 {
	return int64(p.numValues)
}

// NumNulls returns the number of nulls the page contains.
func (p *nilPage) NumNulls() int64 {
	return int64(p.numValues)
}

// Bounds returns the minimum and maximum values of the page, since all values
// in the page are null, both the minimum and maximum values are null.
func (p *nilPage) Bounds() (min, max parquet.Value, ok bool) {
	return parquet.ValueOf(nil).Level(0, 0, p.columnIndex), parquet.ValueOf(nil).Level(0, 0, p.columnIndex), true
}

// Size returns the physical size of the page. Since this page is virtual,
// in-memory and has no real size it returns 0.
func (p *nilPage) Size() int64 {
	// TODO: Return the static size of the struct and its fields instead of 0.
	// While not strictly necessary, it will make the cumulative size of all
	// pages more accurate.
	return 0
}

// Values returns an iterator for all values in the page. All reads will return
// null values with the repetition level and definition level set to 0, and the
// appropriate column index configured.
func (p *nilPage) Values() parquet.ValueReader {
	return &nilValueReader{
		numValues: p.numValues,
		idx:       p.columnIndex,
		read:      p.seek,
	}
}

// nilValueReader s an iterator for all values in the page. All reads will
// return null values with the repetition level and definition level set to 0,
// and the appropriate column index configured.
type nilValueReader struct {
	numValues int
	idx       int
	read      int
}

// min determines the minimum of two integers and returns the minimum.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ReadValues reads the next n values from the page and returns the amount of
// values read. It attempts to write the number of values that the `values`
// parameter can hold. If less values are left to be read than there is space
// for in the `values` parameter, it will return the number of values read and
// an `io.EOF` error. Implements the parquet.ValueReader interface.
func (p *nilValueReader) ReadValues(values []parquet.Value) (int, error) {
	i := 0
	m := min(len(values), p.numValues-p.read)
	for ; i < m; i++ {
		values[i] = parquet.ValueOf(nil).Level(0, 0, p.idx)
	}

	p.read += i
	if p.read >= p.numValues {
		return i, io.EOF
	}
	return i, nil
}

// Buffer returns the nilPage as a parquet.BufferedPage, since the page is
// entirely a virtual construct, it is already considered buffered and just
// returns itself. Implements the parquet.Page interface.
func (p *nilPage) Buffer() parquet.BufferedPage {
	return p
}

// DefinitionLevels returns the definition levels of the page. Since the page
// contains only null values, all of them are 0. Implements the
// parquet.BufferedPage interface.
func (p *nilPage) DefinitionLevels() []byte {
	return nil
}

// RepetitionLevels returns the definition levels of the page. Since the page
// contains only null values, all of them are 0. Implements the
// parquet.BufferedPage interface.
func (p *nilPage) RepetitionLevels() []byte {
	return nil
}

// Slice returns the nilPage with the subset of values represented by the range
// between i and j. Implements the parquet.BufferedPage interface.
func (p *nilPage) Slice(i, j int64) parquet.BufferedPage {
	return &nilPage{
		numValues:   int(j - i),
		columnIndex: p.columnIndex,
	}
}

// Data is unimplemented, since the page is virtual and does not need to be
// written in its current usage in this package. If that changes this method
// needs to be implemented. Implements the parquet.BufferedPage interface.
func (p *nilPage) Data() []byte {
	panic("not implemented")
}

// Clone creates a copy of the nilPage. Implements the parquet.BufferedPage
// interface.
func (p *nilPage) Clone() parquet.BufferedPage {
	return &nilPage{
		numValues:   p.numValues,
		columnIndex: p.columnIndex,
	}
}

// SeekToRow ensures that any page read is positioned at the given row.
// Implements the parquet.Pages interface.
func (p *nilPages) SeekToRow(row int64) error {
	p.seek = int(row)
	return nil
}

// ColumnIndex returns the column index of the column chunk. Since the
// NilColumnChunk is a virtual column chunk only for in-memory purposes, it
// returns nil. Implements the parquet.ColumnChunk interface.
func (c *NilColumnChunk) ColumnIndex() parquet.ColumnIndex {
	return nil
}

// OffsetIndex returns the offset index of the column chunk. Since the
// NilColumnChunk is a virtual column chunk only for in-memory purposes, it
// returns nil. Implements the parquet.ColumnChunk interface.
func (c *NilColumnChunk) OffsetIndex() parquet.OffsetIndex {
	return nil
}

// BloomFilter returns the bloomfilter of the column chunk. Since the
// NilColumnChunk is a virtual column chunk only for in-memory purposes, it
// returns nil. Implements the parquet.ColumnChunk interface.
func (c *NilColumnChunk) BloomFilter() parquet.BloomFilter {
	return nil
}
