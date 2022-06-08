package parquet

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/internal/bits"
)

// Page values represent sequences of parquet values. From the Parquet
// documentation: "Column chunks are a chunk of the data for a particular
// column. They live in a particular row group and are guaranteed to be
// contiguous in the file. Column chunks are divided up into pages. A page is
// conceptually an indivisible unit (in terms of compression and encoding).
// There can be multiple page types which are interleaved in a column chunk."
//
// https://github.com/apache/parquet-format#glossary
type Page interface {
	// Returns the type of values read from this page.
	//
	// The returned type can be used to encode the page data, in the case of
	// an indexed page (which has a dictionary), the type is configured to
	// encode the indexes stored in the page rather than the plain values.
	Type() Type

	// Returns the column index that this page belongs to.
	Column() int

	// If the page contains indexed values, calling this method returns the
	// dictionary in which the values are looked up. Otherwise, the method
	// returns nil.
	Dictionary() Dictionary

	// Returns the number of rows, values, and nulls in the page. The number of
	// rows may be less than the number of values in the page if the page is
	// part of a repeated column.
	NumRows() int64
	NumValues() int64
	NumNulls() int64

	// Returns the min and max values currently buffered in the writer.
	//
	// The third value is a boolean indicating whether the page bounds were
	// available. Page bounds may not be known if the page contained no values
	// or only nulls, or if they were read from a parquet file which had neither
	// page statistics nor a page index.
	Bounds() (min, max Value, ok bool)

	// Returns the size of the page in bytes (uncompressed).
	Size() int64

	// Returns a reader exposing the values contained in the page.
	//
	// Depending on the underlying implementation, the returned reader may
	// support reading an array of typed Go values by implementing interfaces
	// like parquet.Int32Reader. Applications should use type assertions on
	// the returned reader to determine whether those optimizations are
	// available.
	Values() ValueReader

	// Buffer returns the page as a BufferedPage, which may be the page itself
	// if it was already buffered.
	Buffer() BufferedPage
}

// BufferedPage is an extension of the Page interface implemented by pages
// that are buffered in memory.
type BufferedPage interface {
	Page

	// Returns a copy of the page which does not share any of the buffers, but
	// contains the same values, repetition and definition levels.
	Clone() BufferedPage

	// Returns a new page which is as slice of the receiver between row indexes
	// i and j.
	Slice(i, j int64) BufferedPage

	// Expose the lists of repetition and definition levels of the page.
	//
	// The returned slices may be empty when the page has no repetition or
	// definition levels.
	RepetitionLevels() []byte
	DefinitionLevels() []byte

	// Returns the in-memory buffer holding the page values.
	//
	// The buffer has the page values serialized in the PLAIN encoding.
	//
	// The intent is for the returned value to be used as input parameter when
	// calling the Encode method of the associated Type.
	//
	// The returned slice may be the same across multiple calls to this method,
	// applications must treat the content as immutable.
	Data() []byte
}

// CompressedPage is an extension of the Page interface implemented by pages
// that have been compressed to their on-file representation.
type CompressedPage interface {
	Page

	// Returns a representation of the page header.
	PageHeader() PageHeader

	// Returns a reader exposing the content of the compressed page.
	PageData() io.Reader

	// Returns the size of the page data.
	PageSize() int64

	// CRC returns the IEEE CRC32 checksum of the page.
	CRC() uint32
}

// PageReader is an interface implemented by types that support producing a
// sequence of pages.
type PageReader interface {
	// Reads and returns the next page from the sequence. When all pages have
	// been read, or if the sequence was closed, the method returns io.EOF.
	//
	// The returned page and other objects derived from it remain valid until
	// the next call to ReadPage, or until the sequence is closed. The page
	// reader may use this property to optimize resource management by reusing
	// memory across pages. Applications that need to acquire ownership of the
	// returned page must clone by calling page.Buffer().Clone() to create a
	// copy in memory.
	ReadPage() (Page, error)
}

// PageWriter is an interface implemented by types that support writing pages
// to an underlying storage medium.
type PageWriter interface {
	WritePage(Page) (int64, error)
}

// Pages is an interface implemented by page readers returned by calling the
// Pages method of ColumnChunk instances.
type Pages interface {
	PageReader
	RowSeeker
	io.Closer
}

func copyPagesAndClose(w PageWriter, r Pages) (int64, error) {
	defer r.Close()
	return CopyPages(w, r)
}

type singlePage struct {
	page Page
	seek int64
}

func (r *singlePage) ReadPage() (Page, error) {
	if r.page != nil {
		if numRows := r.page.NumRows(); r.seek < numRows {
			seek := r.seek
			r.seek = numRows
			if seek > 0 {
				return r.page.Buffer().Slice(seek, numRows), nil
			}
			return r.page, nil
		}
	}
	return nil, io.EOF
}

func (r *singlePage) SeekToRow(rowIndex int64) error {
	r.seek = rowIndex
	return nil
}

func (r *singlePage) Close() error {
	r.page = nil
	r.seek = 0
	return nil
}

func onePage(page Page) Pages { return &singlePage{page: page} }

// CopyPages copies pages from src to dst, returning the number of values that
// were copied.
//
// The function returns any error it encounters reading or writing pages, except
// for io.EOF from the reader which indicates that there were no more pages to
// read.
func CopyPages(dst PageWriter, src PageReader) (numValues int64, err error) {
	for {
		p, err := src.ReadPage()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return numValues, err
		}
		n, err := dst.WritePage(p)
		numValues += n
		if err != nil {
			return numValues, err
		}
	}
}

func forEachPageSlice(page BufferedPage, wantSize int64, do func(BufferedPage) error) error {
	numRows := page.NumRows()
	if numRows == 0 {
		return nil
	}

	pageSize := page.Size()
	numPages := (pageSize + (wantSize - 1)) / wantSize
	rowIndex := int64(0)
	if numPages < 2 {
		return do(page)
	}

	for numPages > 0 {
		lastRowIndex := rowIndex + ((numRows - rowIndex) / numPages)
		if err := do(page.Slice(rowIndex, lastRowIndex)); err != nil {
			return err
		}
		rowIndex = lastRowIndex
		numPages--
	}

	return nil
}

// errorPage is an implementation of the Page interface which always errors when
// attempting to read its values.
//
// The error page declares that it contains one value (even if it does not)
// as a way to ensure that it is not ignored due to being empty when written
// to a file.
type errorPage struct {
	typ         Type
	err         error
	columnIndex int
}

func newErrorPage(typ Type, columnIndex int, msg string, args ...interface{}) *errorPage {
	return &errorPage{
		typ:         typ,
		err:         fmt.Errorf(msg, args...),
		columnIndex: columnIndex,
	}
}

func (page *errorPage) Type() Type                        { return page.typ }
func (page *errorPage) Column() int                       { return page.columnIndex }
func (page *errorPage) Dictionary() Dictionary            { return nil }
func (page *errorPage) NumRows() int64                    { return 1 }
func (page *errorPage) NumValues() int64                  { return 1 }
func (page *errorPage) NumNulls() int64                   { return 0 }
func (page *errorPage) Bounds() (min, max Value, ok bool) { return }
func (page *errorPage) Clone() BufferedPage               { return page }
func (page *errorPage) Slice(i, j int64) BufferedPage     { return page }
func (page *errorPage) Size() int64                       { return 1 }
func (page *errorPage) RepetitionLevels() []byte          { return nil }
func (page *errorPage) DefinitionLevels() []byte          { return nil }
func (page *errorPage) Data() []byte                      { return nil }
func (page *errorPage) Values() ValueReader               { return errorPageValues{page: page} }
func (page *errorPage) Buffer() BufferedPage              { return page }

type errorPageValues struct{ page *errorPage }

func (r errorPageValues) ReadValues([]Value) (int, error) { return 0, r.page.err }
func (r errorPageValues) Close() error                    { return nil }

func errPageBoundsOutOfRange(i, j, n int64) error {
	return fmt.Errorf("page bounds out of range [%d:%d]: with length %d", i, j, n)
}

func countLevelsEqual(levels []byte, value byte) int {
	return bits.CountByte(levels, value)
}

func countLevelsNotEqual(levels []byte, value byte) int {
	return len(levels) - countLevelsEqual(levels, value)
}

func appendLevel(levels []byte, value byte, count int) []byte {
	if count > 0 {
		i := len(levels)
		n := len(levels) + count

		if cap(levels) < n {
			newLevels := make([]byte, n)
			copy(newLevels, levels)
			levels = newLevels
		} else {
			levels = levels[:n]
		}

		fill := levels[i:]
		for i := range fill {
			fill[i] = value
		}
	}
	return levels
}

type optionalPage struct {
	base               BufferedPage
	maxDefinitionLevel byte
	definitionLevels   []byte
}

func newOptionalPage(base BufferedPage, maxDefinitionLevel byte, definitionLevels []byte) *optionalPage {
	return &optionalPage{
		base:               base,
		maxDefinitionLevel: maxDefinitionLevel,
		definitionLevels:   definitionLevels,
	}
}

func (page *optionalPage) Type() Type { return page.base.Type() }

func (page *optionalPage) Column() int { return page.base.Column() }

func (page *optionalPage) Dictionary() Dictionary { return page.base.Dictionary() }

func (page *optionalPage) NumRows() int64 { return int64(len(page.definitionLevels)) }

func (page *optionalPage) NumValues() int64 { return int64(len(page.definitionLevels)) }

func (page *optionalPage) NumNulls() int64 {
	return int64(countLevelsNotEqual(page.definitionLevels, page.maxDefinitionLevel))
}

func (page *optionalPage) Bounds() (min, max Value, ok bool) { return page.base.Bounds() }

func (page *optionalPage) Size() int64 { return page.base.Size() + int64(len(page.definitionLevels)) }

func (page *optionalPage) RepetitionLevels() []byte { return nil }

func (page *optionalPage) DefinitionLevels() []byte { return page.definitionLevels }

func (page *optionalPage) Data() []byte { return page.base.Data() }

func (page *optionalPage) Values() ValueReader {
	return &optionalPageValues{
		page:   page,
		values: page.base.Values(),
	}
}

func (page *optionalPage) Buffer() BufferedPage { return page }

func (page *optionalPage) Clone() BufferedPage {
	return newOptionalPage(
		page.base.Clone(),
		page.maxDefinitionLevel,
		append([]byte{}, page.definitionLevels...),
	)
}

func (page *optionalPage) Slice(i, j int64) BufferedPage {
	numNulls1 := int64(countLevelsNotEqual(page.definitionLevels[:i], page.maxDefinitionLevel))
	numNulls2 := int64(countLevelsNotEqual(page.definitionLevels[i:j], page.maxDefinitionLevel))
	return newOptionalPage(
		page.base.Slice(i-numNulls1, j-(numNulls1+numNulls2)),
		page.maxDefinitionLevel,
		page.definitionLevels[i:j],
	)
}

type repeatedPage struct {
	base               BufferedPage
	maxRepetitionLevel byte
	maxDefinitionLevel byte
	definitionLevels   []byte
	repetitionLevels   []byte
}

func newRepeatedPage(base BufferedPage, maxRepetitionLevel, maxDefinitionLevel byte, repetitionLevels, definitionLevels []byte) *repeatedPage {
	return &repeatedPage{
		base:               base,
		maxRepetitionLevel: maxRepetitionLevel,
		maxDefinitionLevel: maxDefinitionLevel,
		definitionLevels:   definitionLevels,
		repetitionLevels:   repetitionLevels,
	}
}

func (page *repeatedPage) Type() Type { return page.base.Type() }

func (page *repeatedPage) Column() int { return page.base.Column() }

func (page *repeatedPage) Dictionary() Dictionary { return page.base.Dictionary() }

func (page *repeatedPage) NumRows() int64 { return int64(countLevelsEqual(page.repetitionLevels, 0)) }

func (page *repeatedPage) NumValues() int64 { return int64(len(page.definitionLevels)) }

func (page *repeatedPage) NumNulls() int64 {
	return int64(countLevelsNotEqual(page.definitionLevels, page.maxDefinitionLevel))
}

func (page *repeatedPage) Bounds() (min, max Value, ok bool) { return page.base.Bounds() }

func (page *repeatedPage) Size() int64 {
	return int64(len(page.repetitionLevels)) + int64(len(page.definitionLevels)) + page.base.Size()
}

func (page *repeatedPage) RepetitionLevels() []byte { return page.repetitionLevels }

func (page *repeatedPage) DefinitionLevels() []byte { return page.definitionLevels }

func (page *repeatedPage) Data() []byte { return page.base.Data() }

func (page *repeatedPage) Values() ValueReader {
	return &repeatedPageValues{
		page:   page,
		values: page.base.Values(),
	}
}

func (page *repeatedPage) Buffer() BufferedPage { return page }

func (page *repeatedPage) Clone() BufferedPage {
	return newRepeatedPage(
		page.base.Clone(),
		page.maxRepetitionLevel,
		page.maxDefinitionLevel,
		append([]byte{}, page.repetitionLevels...),
		append([]byte{}, page.definitionLevels...),
	)
}

func (page *repeatedPage) Slice(i, j int64) BufferedPage {
	numRows := page.NumRows()
	if i < 0 || i > numRows {
		panic(errPageBoundsOutOfRange(i, j, numRows))
	}
	if j < 0 || j > numRows {
		panic(errPageBoundsOutOfRange(i, j, numRows))
	}
	if i > j {
		panic(errPageBoundsOutOfRange(i, j, numRows))
	}

	rowIndex0 := 0
	rowIndex1 := len(page.repetitionLevels)
	rowIndex2 := len(page.repetitionLevels)

	for k, def := range page.repetitionLevels {
		if def == 0 {
			if rowIndex0 == int(i) {
				rowIndex1 = k
				break
			}
			rowIndex0++
		}
	}

	for k, def := range page.repetitionLevels[rowIndex1:] {
		if def == 0 {
			if rowIndex0 == int(j) {
				rowIndex2 = rowIndex1 + k
				break
			}
			rowIndex0++
		}
	}

	numNulls1 := countLevelsNotEqual(page.definitionLevels[:rowIndex1], page.maxDefinitionLevel)
	numNulls2 := countLevelsNotEqual(page.definitionLevels[rowIndex1:rowIndex2], page.maxDefinitionLevel)

	i = int64(rowIndex1 - numNulls1)
	j = int64(rowIndex2 - (numNulls1 + numNulls2))

	return newRepeatedPage(
		page.base.Slice(i, j),
		page.maxRepetitionLevel,
		page.maxDefinitionLevel,
		page.repetitionLevels[rowIndex1:rowIndex2],
		page.definitionLevels[rowIndex1:rowIndex2],
	)
}

type booleanPage struct {
	typ         Type
	bits        []byte
	offset      int32
	numValues   int32
	columnIndex int16
}

func newBooleanPage(typ Type, columnIndex int16, numValues int32, values []byte) *booleanPage {
	return &booleanPage{
		typ:         typ,
		bits:        values[:bits.ByteCount(uint(numValues))],
		numValues:   numValues,
		columnIndex: ^columnIndex,
	}
}

func (page *booleanPage) Type() Type { return page.typ }

func (page *booleanPage) Column() int { return int(^page.columnIndex) }

func (page *booleanPage) Dictionary() Dictionary { return nil }

func (page *booleanPage) NumRows() int64 { return int64(page.numValues) }

func (page *booleanPage) NumValues() int64 { return int64(page.numValues) }

func (page *booleanPage) NumNulls() int64 { return 0 }

func (page *booleanPage) Size() int64 { return int64(len(page.bits)) }

func (page *booleanPage) RepetitionLevels() []byte { return nil }

func (page *booleanPage) DefinitionLevels() []byte { return nil }

func (page *booleanPage) Data() []byte { return page.bits }

func (page *booleanPage) Values() ValueReader { return &booleanPageValues{page: page} }

func (page *booleanPage) Buffer() BufferedPage { return page }

func (page *booleanPage) valueAt(i int) bool {
	j := uint32(int(page.offset)+i) / 8
	k := uint32(int(page.offset)+i) % 8
	return ((page.bits[j] >> k) & 1) != 0
}

func (page *booleanPage) min() bool {
	for i := 0; i < int(page.numValues); i++ {
		if !page.valueAt(i) {
			return false
		}
	}
	return page.numValues > 0
}

func (page *booleanPage) max() bool {
	for i := 0; i < int(page.numValues); i++ {
		if page.valueAt(i) {
			return true
		}
	}
	return false
}

func (page *booleanPage) bounds() (min, max bool) {
	hasFalse, hasTrue := false, false

	for i := 0; i < int(page.numValues); i++ {
		v := page.valueAt(i)
		if v {
			hasTrue = true
		} else {
			hasFalse = true
		}
		if hasTrue && hasFalse {
			break
		}
	}

	min = !hasFalse
	max = hasTrue
	return min, max
}

func (page *booleanPage) Bounds() (min, max Value, ok bool) {
	if ok = page.numValues > 0; ok {
		minBool, maxBool := page.bounds()
		min = makeValueBoolean(minBool)
		max = makeValueBoolean(maxBool)
	}
	return min, max, ok
}

func (page *booleanPage) Clone() BufferedPage {
	return &booleanPage{
		typ:         page.typ,
		bits:        append([]byte{}, page.bits...),
		offset:      page.offset,
		numValues:   page.numValues,
		columnIndex: page.columnIndex,
	}
}

func (page *booleanPage) Slice(i, j int64) BufferedPage {
	off := i / 8
	end := j / 8

	if (j % 8) != 0 {
		end++
	}

	return &booleanPage{
		typ:         page.typ,
		bits:        page.bits[off:end],
		offset:      int32(i % 8),
		numValues:   int32(j - i),
		columnIndex: page.columnIndex,
	}
}

type int32Page struct {
	typ         Type
	values      []int32
	columnIndex int16
}

func newInt32Page(typ Type, columnIndex int16, numValues int32, values []byte) *int32Page {
	return &int32Page{
		typ:         typ,
		values:      bits.BytesToInt32(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *int32Page) Type() Type { return page.typ }

func (page *int32Page) Column() int { return int(^page.columnIndex) }

func (page *int32Page) Dictionary() Dictionary { return nil }

func (page *int32Page) NumRows() int64 { return int64(len(page.values)) }

func (page *int32Page) NumValues() int64 { return int64(len(page.values)) }

func (page *int32Page) NumNulls() int64 { return 0 }

func (page *int32Page) Size() int64 { return 4 * int64(len(page.values)) }

func (page *int32Page) RepetitionLevels() []byte { return nil }

func (page *int32Page) DefinitionLevels() []byte { return nil }

func (page *int32Page) Data() []byte { return bits.Int32ToBytes(page.values) }

func (page *int32Page) Values() ValueReader { return &int32PageValues{page: page} }

func (page *int32Page) Buffer() BufferedPage { return page }

func (page *int32Page) min() int32 { return bits.MinInt32(page.values) }

func (page *int32Page) max() int32 { return bits.MaxInt32(page.values) }

func (page *int32Page) bounds() (min, max int32) { return bits.MinMaxInt32(page.values) }

func (page *int32Page) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minInt32, maxInt32 := page.bounds()
		min = makeValueInt32(minInt32)
		max = makeValueInt32(maxInt32)
	}
	return min, max, ok
}

func (page *int32Page) Clone() BufferedPage {
	return &int32Page{
		typ:         page.typ,
		values:      append([]int32{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *int32Page) Slice(i, j int64) BufferedPage {
	return &int32Page{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type int64Page struct {
	typ         Type
	values      []int64
	columnIndex int16
}

func newInt64Page(typ Type, columnIndex int16, numValues int32, values []byte) *int64Page {
	return &int64Page{
		typ:         typ,
		values:      bits.BytesToInt64(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *int64Page) Type() Type { return page.typ }

func (page *int64Page) Column() int { return int(^page.columnIndex) }

func (page *int64Page) Dictionary() Dictionary { return nil }

func (page *int64Page) NumRows() int64 { return int64(len(page.values)) }

func (page *int64Page) NumValues() int64 { return int64(len(page.values)) }

func (page *int64Page) NumNulls() int64 { return 0 }

func (page *int64Page) Size() int64 { return 8 * int64(len(page.values)) }

func (page *int64Page) RepetitionLevels() []byte { return nil }

func (page *int64Page) DefinitionLevels() []byte { return nil }

func (page *int64Page) Data() []byte { return bits.Int64ToBytes(page.values) }

func (page *int64Page) Values() ValueReader { return &int64PageValues{page: page} }

func (page *int64Page) Buffer() BufferedPage { return page }

func (page *int64Page) min() int64 { return bits.MinInt64(page.values) }

func (page *int64Page) max() int64 { return bits.MaxInt64(page.values) }

func (page *int64Page) bounds() (min, max int64) { return bits.MinMaxInt64(page.values) }

func (page *int64Page) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minInt64, maxInt64 := page.bounds()
		min = makeValueInt64(minInt64)
		max = makeValueInt64(maxInt64)
	}
	return min, max, ok
}

func (page *int64Page) Clone() BufferedPage {
	return &int64Page{
		typ:         page.typ,
		values:      append([]int64{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *int64Page) Slice(i, j int64) BufferedPage {
	return &int64Page{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type int96Page struct {
	typ         Type
	values      []deprecated.Int96
	columnIndex int16
}

func newInt96Page(typ Type, columnIndex int16, numValues int32, values []byte) *int96Page {
	return &int96Page{
		typ:         typ,
		values:      deprecated.BytesToInt96(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *int96Page) Type() Type { return page.typ }

func (page *int96Page) Column() int { return int(^page.columnIndex) }

func (page *int96Page) Dictionary() Dictionary { return nil }

func (page *int96Page) NumRows() int64 { return int64(len(page.values)) }

func (page *int96Page) NumValues() int64 { return int64(len(page.values)) }

func (page *int96Page) NumNulls() int64 { return 0 }

func (page *int96Page) Size() int64 { return 12 * int64(len(page.values)) }

func (page *int96Page) RepetitionLevels() []byte { return nil }

func (page *int96Page) DefinitionLevels() []byte { return nil }

func (page *int96Page) Data() []byte { return deprecated.Int96ToBytes(page.values) }

func (page *int96Page) Values() ValueReader { return &int96PageValues{page: page} }

func (page *int96Page) Buffer() BufferedPage { return page }

func (page *int96Page) min() deprecated.Int96 { return deprecated.MinInt96(page.values) }

func (page *int96Page) max() deprecated.Int96 { return deprecated.MaxInt96(page.values) }

func (page *int96Page) bounds() (min, max deprecated.Int96) {
	return deprecated.MinMaxInt96(page.values)
}

func (page *int96Page) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minInt96, maxInt96 := page.bounds()
		min = makeValueInt96(minInt96)
		max = makeValueInt96(maxInt96)
	}
	return min, max, ok
}

func (page *int96Page) Clone() BufferedPage {
	return &int96Page{
		typ:         page.typ,
		values:      append([]deprecated.Int96{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *int96Page) Slice(i, j int64) BufferedPage {
	return &int96Page{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type floatPage struct {
	typ         Type
	values      []float32
	columnIndex int16
}

func newFloatPage(typ Type, columnIndex int16, numValues int32, values []byte) *floatPage {
	return &floatPage{
		typ:         typ,
		values:      bits.BytesToFloat32(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *floatPage) Type() Type { return page.typ }

func (page *floatPage) Column() int { return int(^page.columnIndex) }

func (page *floatPage) Dictionary() Dictionary { return nil }

func (page *floatPage) NumRows() int64 { return int64(len(page.values)) }

func (page *floatPage) NumValues() int64 { return int64(len(page.values)) }

func (page *floatPage) NumNulls() int64 { return 0 }

func (page *floatPage) Size() int64 { return 4 * int64(len(page.values)) }

func (page *floatPage) RepetitionLevels() []byte { return nil }

func (page *floatPage) DefinitionLevels() []byte { return nil }

func (page *floatPage) Data() []byte { return bits.Float32ToBytes(page.values) }

func (page *floatPage) Values() ValueReader { return &floatPageValues{page: page} }

func (page *floatPage) Buffer() BufferedPage { return page }

func (page *floatPage) min() float32 { return bits.MinFloat32(page.values) }

func (page *floatPage) max() float32 { return bits.MaxFloat32(page.values) }

func (page *floatPage) bounds() (min, max float32) { return bits.MinMaxFloat32(page.values) }

func (page *floatPage) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minFloat32, maxFloat32 := page.bounds()
		min = makeValueFloat(minFloat32)
		max = makeValueFloat(maxFloat32)
	}
	return min, max, ok
}

func (page *floatPage) Clone() BufferedPage {
	return &floatPage{
		typ:         page.typ,
		values:      append([]float32{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *floatPage) Slice(i, j int64) BufferedPage {
	return &floatPage{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type doublePage struct {
	typ         Type
	values      []float64
	columnIndex int16
}

func newDoublePage(typ Type, columnIndex int16, numValues int32, values []byte) *doublePage {
	return &doublePage{
		typ:         typ,
		values:      bits.BytesToFloat64(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *doublePage) Type() Type { return page.typ }

func (page *doublePage) Column() int { return int(^page.columnIndex) }

func (page *doublePage) Dictionary() Dictionary { return nil }

func (page *doublePage) NumRows() int64 { return int64(len(page.values)) }

func (page *doublePage) NumValues() int64 { return int64(len(page.values)) }

func (page *doublePage) NumNulls() int64 { return 0 }

func (page *doublePage) Size() int64 { return 8 * int64(len(page.values)) }

func (page *doublePage) RepetitionLevels() []byte { return nil }

func (page *doublePage) DefinitionLevels() []byte { return nil }

func (page *doublePage) Data() []byte { return bits.Float64ToBytes(page.values) }

func (page *doublePage) Values() ValueReader { return &doublePageValues{page: page} }

func (page *doublePage) Buffer() BufferedPage { return page }

func (page *doublePage) min() float64 { return bits.MinFloat64(page.values) }

func (page *doublePage) max() float64 { return bits.MaxFloat64(page.values) }

func (page *doublePage) bounds() (min, max float64) { return bits.MinMaxFloat64(page.values) }

func (page *doublePage) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minFloat64, maxFloat64 := page.bounds()
		min = makeValueDouble(minFloat64)
		max = makeValueDouble(maxFloat64)
	}
	return min, max, ok
}

func (page *doublePage) Clone() BufferedPage {
	return &doublePage{
		typ:         page.typ,
		values:      append([]float64{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *doublePage) Slice(i, j int64) BufferedPage {
	return &doublePage{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type byteArrayPage struct {
	typ         Type
	values      []byte
	numValues   int32
	columnIndex int16
}

func newByteArrayPage(typ Type, columnIndex int16, numValues int32, values []byte) *byteArrayPage {
	return &byteArrayPage{
		typ:         typ,
		values:      values,
		numValues:   numValues,
		columnIndex: ^columnIndex,
	}
}

func (page *byteArrayPage) Type() Type { return page.typ }

func (page *byteArrayPage) Column() int { return int(^page.columnIndex) }

func (page *byteArrayPage) Dictionary() Dictionary { return nil }

func (page *byteArrayPage) NumRows() int64 { return int64(page.numValues) }

func (page *byteArrayPage) NumValues() int64 { return int64(page.numValues) }

func (page *byteArrayPage) NumNulls() int64 { return 0 }

func (page *byteArrayPage) Size() int64 { return int64(len(page.values)) }

func (page *byteArrayPage) RepetitionLevels() []byte { return nil }

func (page *byteArrayPage) DefinitionLevels() []byte { return nil }

func (page *byteArrayPage) Data() []byte { return page.values }

func (page *byteArrayPage) Values() ValueReader { return &byteArrayPageValues{page: page} }

func (page *byteArrayPage) Buffer() BufferedPage { return page }

func (page *byteArrayPage) valueAt(offset uint32) []byte {
	length := binary.LittleEndian.Uint32(page.values[offset:])
	j := 4 + offset
	k := 4 + offset + length
	return page.values[j:k:k]
}

func (page *byteArrayPage) min() (min []byte) {
	if len(page.values) > 0 {
		min = page.valueAt(0)

		for i := 4 + len(min); i < len(page.values); {
			v := page.valueAt(uint32(i))

			if string(v) < string(min) {
				min = v
			}

			i += 4
			i += len(v)
		}
	}
	return min
}

func (page *byteArrayPage) max() (max []byte) {
	if len(page.values) > 0 {
		max = page.valueAt(0)

		for i := 4 + len(max); i < len(page.values); {
			v := page.valueAt(uint32(i))

			if string(v) > string(max) {
				max = v
			}

			i += 4
			i += len(v)
		}
	}
	return max
}

func (page *byteArrayPage) bounds() (min, max []byte) {
	if len(page.values) > 0 {
		min = page.valueAt(0)
		max = min

		for i := 4 + len(min); i < len(page.values); {
			v := page.valueAt(uint32(i))

			switch {
			case string(v) < string(min):
				min = v
			case string(v) > string(max):
				max = v
			}

			i += 4
			i += len(v)
		}
	}
	return min, max
}

func (page *byteArrayPage) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minBytes, maxBytes := page.bounds()
		min = makeValueBytes(ByteArray, minBytes)
		max = makeValueBytes(ByteArray, maxBytes)
	}
	return min, max, ok
}

func (page *byteArrayPage) cloneValues() []byte {
	values := make([]byte, len(page.values))
	copy(values, page.values)
	return values
}

func (page *byteArrayPage) Clone() BufferedPage {
	return &byteArrayPage{
		typ:         page.typ,
		values:      page.cloneValues(),
		numValues:   page.numValues,
		columnIndex: page.columnIndex,
	}
}

func (page *byteArrayPage) Slice(i, j int64) BufferedPage {
	numValues := j - i

	off0 := uint32(0)
	for i > 0 {
		off0 += binary.LittleEndian.Uint32(page.values[off0:])
		off0 += plain.ByteArrayLengthSize
		i--
		j--
	}

	off1 := off0
	for j > 0 {
		off1 += binary.LittleEndian.Uint32(page.values[off1:])
		off1 += plain.ByteArrayLengthSize
		j--
	}

	return &byteArrayPage{
		typ:         page.typ,
		values:      page.values[off0:off1:off1],
		numValues:   int32(numValues),
		columnIndex: page.columnIndex,
	}
}

type fixedLenByteArrayPage struct {
	typ         Type
	data        []byte
	size        int
	columnIndex int16
}

func newFixedLenByteArrayPage(typ Type, columnIndex int16, numValues int32, data []byte) *fixedLenByteArrayPage {
	size := typ.Length()
	if (len(data) % size) != 0 {
		panic("cannot create fixed-length byte array page from input which is not a multiple of the type size")
	}
	if int(numValues) != len(data)/size {
		panic(fmt.Errorf("number of values mismatch in numValues and data arguments: %d != %d", numValues, len(data)/size))
	}
	return &fixedLenByteArrayPage{
		typ:         typ,
		data:        data,
		size:        size,
		columnIndex: ^columnIndex,
	}
}

func (page *fixedLenByteArrayPage) Type() Type { return page.typ }

func (page *fixedLenByteArrayPage) Column() int { return int(^page.columnIndex) }

func (page *fixedLenByteArrayPage) Dictionary() Dictionary { return nil }

func (page *fixedLenByteArrayPage) NumRows() int64 { return int64(len(page.data) / page.size) }

func (page *fixedLenByteArrayPage) NumValues() int64 { return int64(len(page.data) / page.size) }

func (page *fixedLenByteArrayPage) NumNulls() int64 { return 0 }

func (page *fixedLenByteArrayPage) Size() int64 { return int64(len(page.data)) }

func (page *fixedLenByteArrayPage) RepetitionLevels() []byte { return nil }

func (page *fixedLenByteArrayPage) DefinitionLevels() []byte { return nil }

func (page *fixedLenByteArrayPage) Data() []byte { return page.data }

func (page *fixedLenByteArrayPage) Values() ValueReader {
	return &fixedLenByteArrayPageValues{page: page}
}

func (page *fixedLenByteArrayPage) Buffer() BufferedPage { return page }

func (page *fixedLenByteArrayPage) min() []byte {
	return bits.MinFixedLenByteArray(page.size, page.data)
}

func (page *fixedLenByteArrayPage) max() []byte {
	return bits.MaxFixedLenByteArray(page.size, page.data)
}

func (page *fixedLenByteArrayPage) bounds() (min, max []byte) {
	return bits.MinMaxFixedLenByteArray(page.size, page.data)
}

func (page *fixedLenByteArrayPage) Bounds() (min, max Value, ok bool) {
	if ok = len(page.data) > 0; ok {
		minBytes, maxBytes := page.bounds()
		min = makeValueBytes(FixedLenByteArray, minBytes)
		max = makeValueBytes(FixedLenByteArray, maxBytes)
	}
	return min, max, ok
}

func (page *fixedLenByteArrayPage) Clone() BufferedPage {
	return &fixedLenByteArrayPage{
		typ:         page.typ,
		data:        append([]byte{}, page.data...),
		size:        page.size,
		columnIndex: page.columnIndex,
	}
}

func (page *fixedLenByteArrayPage) Slice(i, j int64) BufferedPage {
	return &fixedLenByteArrayPage{
		typ:         page.typ,
		data:        page.data[i*int64(page.size) : j*int64(page.size)],
		size:        page.size,
		columnIndex: page.columnIndex,
	}
}

type uint32Page struct {
	typ         Type
	values      []uint32
	columnIndex int16
}

func newUint32Page(typ Type, columnIndex int16, numValues int32, values []byte) *uint32Page {
	return &uint32Page{
		typ:         typ,
		values:      bits.BytesToUint32(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *uint32Page) Type() Type { return page.typ }

func (page *uint32Page) Column() int { return int(^page.columnIndex) }

func (page *uint32Page) Dictionary() Dictionary { return nil }

func (page *uint32Page) NumRows() int64 { return int64(len(page.values)) }

func (page *uint32Page) NumValues() int64 { return int64(len(page.values)) }

func (page *uint32Page) NumNulls() int64 { return 0 }

func (page *uint32Page) Size() int64 { return 4 * int64(len(page.values)) }

func (page *uint32Page) RepetitionLevels() []byte { return nil }

func (page *uint32Page) DefinitionLevels() []byte { return nil }

func (page *uint32Page) Data() []byte { return bits.Uint32ToBytes(page.values) }

func (page *uint32Page) Values() ValueReader { return &uint32PageValues{page: page} }

func (page *uint32Page) Buffer() BufferedPage { return page }

func (page *uint32Page) min() uint32 { return bits.MinUint32(page.values) }

func (page *uint32Page) max() uint32 { return bits.MaxUint32(page.values) }

func (page *uint32Page) bounds() (min, max uint32) { return bits.MinMaxUint32(page.values) }

func (page *uint32Page) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minUint32, maxUint32 := page.bounds()
		min = makeValueUint32(minUint32)
		max = makeValueUint32(maxUint32)
	}
	return min, max, ok
}

func (page *uint32Page) Clone() BufferedPage {
	return &uint32Page{
		typ:         page.typ,
		values:      append([]uint32{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *uint32Page) Slice(i, j int64) BufferedPage {
	return &uint32Page{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type uint64Page struct {
	typ         Type
	values      []uint64
	columnIndex int16
}

func newUint64Page(typ Type, columnIndex int16, numValues int32, values []byte) *uint64Page {
	return &uint64Page{
		typ:         typ,
		values:      bits.BytesToUint64(values)[:numValues],
		columnIndex: ^columnIndex,
	}
}

func (page *uint64Page) Type() Type { return page.typ }

func (page *uint64Page) Column() int { return int(^page.columnIndex) }

func (page *uint64Page) Dictionary() Dictionary { return nil }

func (page *uint64Page) NumRows() int64 { return int64(len(page.values)) }

func (page *uint64Page) NumValues() int64 { return int64(len(page.values)) }

func (page *uint64Page) NumNulls() int64 { return 0 }

func (page *uint64Page) Size() int64 { return 8 * int64(len(page.values)) }

func (page *uint64Page) RepetitionLevels() []byte { return nil }

func (page *uint64Page) DefinitionLevels() []byte { return nil }

func (page *uint64Page) Data() []byte { return bits.Uint64ToBytes(page.values) }

func (page *uint64Page) Values() ValueReader { return &uint64PageValues{page: page} }

func (page *uint64Page) Buffer() BufferedPage { return page }

func (page *uint64Page) min() uint64 { return bits.MinUint64(page.values) }

func (page *uint64Page) max() uint64 { return bits.MaxUint64(page.values) }

func (page *uint64Page) bounds() (min, max uint64) { return bits.MinMaxUint64(page.values) }

func (page *uint64Page) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		minUint64, maxUint64 := page.bounds()
		min = makeValueUint64(minUint64)
		max = makeValueUint64(maxUint64)
	}
	return min, max, ok
}

func (page *uint64Page) Clone() BufferedPage {
	return &uint64Page{
		typ:         page.typ,
		values:      append([]uint64{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *uint64Page) Slice(i, j int64) BufferedPage {
	return &uint64Page{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

type nullPage struct {
	typ    Type
	column int
	count  int
}

func newNullPage(typ Type, columnIndex int16, numValues int32) *nullPage {
	return &nullPage{
		typ:    typ,
		column: int(columnIndex),
		count:  int(numValues),
	}
}

func (page *nullPage) Type() Type                        { return page.typ }
func (page *nullPage) Column() int                       { return page.column }
func (page *nullPage) Dictionary() Dictionary            { return nil }
func (page *nullPage) NumRows() int64                    { return int64(page.count) }
func (page *nullPage) NumValues() int64                  { return int64(page.count) }
func (page *nullPage) NumNulls() int64                   { return int64(page.count) }
func (page *nullPage) Bounds() (min, max Value, ok bool) { return }
func (page *nullPage) Size() int64                       { return 1 }
func (page *nullPage) Values() ValueReader {
	return &nullPageValues{column: page.column, remain: page.count}
}
func (page *nullPage) Buffer() BufferedPage { return page }
func (page *nullPage) Clone() BufferedPage  { return page }
func (page *nullPage) Slice(i, j int64) BufferedPage {
	return &nullPage{column: page.column, count: page.count - int(j-i)}
}
func (page *nullPage) RepetitionLevels() []byte { return nil }
func (page *nullPage) DefinitionLevels() []byte { return nil }
func (page *nullPage) Data() []byte             { return nil }
