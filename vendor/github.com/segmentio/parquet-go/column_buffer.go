package parquet

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"unsafe"

	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/internal/bits"
)

// ColumnBuffer is an interface representing columns of a row group.
//
// ColumnBuffer implements sort.Interface as a way to support reordering the
// rows that have been written to it.
type ColumnBuffer interface {
	// Exposes a read-only view of the column buffer.
	ColumnChunk

	// The column implements ValueReaderAt as a mechanism to read values at
	// specific locations within the buffer.
	ValueReaderAt

	// The column implements ValueWriter as a mechanism to optimize the copy
	// of values into the buffer in contexts where the row information is
	// provided by the values because the repetition and definition levels
	// are set.
	ValueWriter

	// For indexed columns, returns the underlying dictionary holding the column
	// values. If the column is not indexed, nil is returned.
	Dictionary() Dictionary

	// Returns a copy of the column. The returned copy shares no memory with
	// the original, mutations of either column will not modify the other.
	Clone() ColumnBuffer

	// Returns the column as a BufferedPage.
	Page() BufferedPage

	// Clears all rows written to the column.
	Reset()

	// Returns the current capacity of the column (rows).
	Cap() int

	// Returns the number of rows currently written to the column.
	Len() int

	// Compares rows at index i and j and reports whether i < j.
	Less(i, j int) bool

	// Swaps rows at index i and j.
	Swap(i, j int)

	// Returns the size of the column buffer in bytes.
	Size() int64
}

type array struct {
	ptr unsafe.Pointer
	len int
}

func (a array) index(i int, size, offset uintptr) unsafe.Pointer {
	return unsafe.Add(a.ptr, uintptr(i)*size+offset)
}

func columnIndexOfNullable(base ColumnBuffer, maxDefinitionLevel byte, definitionLevels []byte) ColumnIndex {
	return &nullableColumnIndex{
		ColumnIndex:        base.ColumnIndex(),
		maxDefinitionLevel: maxDefinitionLevel,
		definitionLevels:   definitionLevels,
	}
}

type nullableColumnIndex struct {
	ColumnIndex
	maxDefinitionLevel byte
	definitionLevels   []byte
}

func (index *nullableColumnIndex) NullPage(i int) bool {
	return index.NullCount(i) == int64(len(index.definitionLevels))
}

func (index *nullableColumnIndex) NullCount(i int) int64 {
	return int64(countLevelsNotEqual(index.definitionLevels, index.maxDefinitionLevel))
}

type nullOrdering func(column ColumnBuffer, i, j int, maxDefinitionLevel, definitionLevel1, definitionLevel2 byte) bool

func nullsGoFirst(column ColumnBuffer, i, j int, maxDefinitionLevel, definitionLevel1, definitionLevel2 byte) bool {
	if definitionLevel1 != maxDefinitionLevel {
		return definitionLevel2 == maxDefinitionLevel
	} else {
		return definitionLevel2 == maxDefinitionLevel && column.Less(i, j)
	}
}

func nullsGoLast(column ColumnBuffer, i, j int, maxDefinitionLevel, definitionLevel1, definitionLevel2 byte) bool {
	return definitionLevel1 == maxDefinitionLevel && (definitionLevel2 != maxDefinitionLevel || column.Less(i, j))
}

// reversedColumnBuffer is an adapter of ColumnBuffer which inverses the order
// in which rows are ordered when the column gets sorted.
//
// This type is used when buffers are constructed with sorting columns ordering
// values in descending order.
type reversedColumnBuffer struct{ ColumnBuffer }

func (col *reversedColumnBuffer) Less(i, j int) bool { return col.ColumnBuffer.Less(j, i) }

// optionalColumnBuffer is an implementation of the ColumnBuffer interface used
// as a wrapper to an underlying ColumnBuffer to manage the creation of
// definition levels.
//
// Null values are not written to the underlying column; instead, the buffer
// tracks offsets of row values in the column, null row values are represented
// by the value -1 and a definition level less than the max.
//
// This column buffer type is used for all leaf columns that have a non-zero
// max definition level and a zero repetition level, which may be because the
// column or one of its parent(s) are marked optional.
type optionalColumnBuffer struct {
	base               ColumnBuffer
	maxDefinitionLevel byte
	rows               []int32
	sortIndex          []int32
	definitionLevels   []byte
	nullOrdering       nullOrdering
}

func newOptionalColumnBuffer(base ColumnBuffer, maxDefinitionLevel byte, nullOrdering nullOrdering) *optionalColumnBuffer {
	n := base.Cap()
	return &optionalColumnBuffer{
		base:               base,
		maxDefinitionLevel: maxDefinitionLevel,
		rows:               make([]int32, 0, n),
		definitionLevels:   make([]byte, 0, n),
		nullOrdering:       nullOrdering,
	}
}

func (col *optionalColumnBuffer) Clone() ColumnBuffer {
	return &optionalColumnBuffer{
		base:               col.base.Clone(),
		maxDefinitionLevel: col.maxDefinitionLevel,
		rows:               append([]int32{}, col.rows...),
		definitionLevels:   append([]byte{}, col.definitionLevels...),
		nullOrdering:       col.nullOrdering,
	}
}

func (col *optionalColumnBuffer) Type() Type {
	return col.base.Type()
}

func (col *optionalColumnBuffer) NumValues() int64 {
	return int64(len(col.definitionLevels))
}

func (col *optionalColumnBuffer) ColumnIndex() ColumnIndex {
	return columnIndexOfNullable(col.base, col.maxDefinitionLevel, col.definitionLevels)
}

func (col *optionalColumnBuffer) OffsetIndex() OffsetIndex {
	return col.base.OffsetIndex()
}

func (col *optionalColumnBuffer) BloomFilter() BloomFilter {
	return col.base.BloomFilter()
}

func (col *optionalColumnBuffer) Dictionary() Dictionary {
	return col.base.Dictionary()
}

func (col *optionalColumnBuffer) Column() int {
	return col.base.Column()
}

func (col *optionalColumnBuffer) Pages() Pages {
	return onePage(col.Page())
}

func (col *optionalColumnBuffer) Page() BufferedPage {
	if !optionalRowsHaveBeenReordered(col.rows) {
		// No need for any cyclic sorting if the rows have not been reordered.
		// This case is also important because the cyclic sorting modifies the
		// buffer which makes it unsafe to read the buffer concurrently.
		return newOptionalPage(col.base.Page(), col.maxDefinitionLevel, col.definitionLevels)
	}

	numNulls := countLevelsNotEqual(col.definitionLevels, col.maxDefinitionLevel)
	numValues := len(col.rows) - numNulls

	if numValues > 0 {
		if cap(col.sortIndex) < numValues {
			col.sortIndex = make([]int32, numValues)
		}
		sortIndex := col.sortIndex[:numValues]
		i := 0
		for _, j := range col.rows {
			if j >= 0 {
				sortIndex[j] = int32(i)
				i++
			}
		}

		// Cyclic sort: O(N)
		for i := range sortIndex {
			for j := int(sortIndex[i]); i != j; j = int(sortIndex[i]) {
				col.base.Swap(i, j)
				sortIndex[i], sortIndex[j] = sortIndex[j], sortIndex[i]
			}
		}
	}

	i := 0
	for _, r := range col.rows {
		if r >= 0 {
			col.rows[i] = int32(i)
			i++
		}
	}

	return newOptionalPage(col.base.Page(), col.maxDefinitionLevel, col.definitionLevels)
}

func (col *optionalColumnBuffer) Reset() {
	col.base.Reset()
	col.rows = col.rows[:0]
	col.definitionLevels = col.definitionLevels[:0]
}

func (col *optionalColumnBuffer) Size() int64 {
	return int64(4*len(col.rows)+4*len(col.sortIndex)+len(col.definitionLevels)) + col.base.Size()
}

func (col *optionalColumnBuffer) Cap() int { return cap(col.rows) }

func (col *optionalColumnBuffer) Len() int { return len(col.rows) }

func (col *optionalColumnBuffer) Less(i, j int) bool {
	return col.nullOrdering(
		col.base,
		int(col.rows[i]),
		int(col.rows[j]),
		col.maxDefinitionLevel,
		col.definitionLevels[i],
		col.definitionLevels[j],
	)
}

func (col *optionalColumnBuffer) Swap(i, j int) {
	// Because the underlying column does not contain null values, we cannot
	// swap its values at indexes i and j. We swap the row indexes only, then
	// reorder the underlying buffer using a cyclic sort when the buffer is
	// materialized into a page view.
	col.rows[i], col.rows[j] = col.rows[j], col.rows[i]
	col.definitionLevels[i], col.definitionLevels[j] = col.definitionLevels[j], col.definitionLevels[i]
}

func (col *optionalColumnBuffer) WriteValues(values []Value) (n int, err error) {
	rowIndex := int32(col.base.Len())

	for n < len(values) {
		// Collect index range of contiguous null values, from i to n. If this
		// for loop exhausts the values, all remaining if statements and for
		// loops will be no-ops and the loop will terminate.
		i := n
		for n < len(values) && values[n].definitionLevel != col.maxDefinitionLevel {
			n++
		}

		// Write the contiguous null values up until the first non-null value
		// obtained in the for loop above.
		for _, v := range values[i:n] {
			col.rows = append(col.rows, -1)
			col.definitionLevels = append(col.definitionLevels, v.definitionLevel)
		}

		// Collect index range of contiguous non-null values, from i to n.
		i = n
		for n < len(values) && values[n].definitionLevel == col.maxDefinitionLevel {
			n++
		}

		// As long as i < n we have non-null values still to write. It is
		// possible that we just exhausted the input values in which case i == n
		// and the outer for loop will terminate.
		if i < n {
			count, err := col.base.WriteValues(values[i:n])
			col.definitionLevels = appendLevel(col.definitionLevels, col.maxDefinitionLevel, count)

			for count > 0 {
				col.rows = append(col.rows, rowIndex)
				rowIndex++
				count--
			}

			if err != nil {
				return n, err
			}
		}
	}

	return n, nil
}

func (col *optionalColumnBuffer) ReadValuesAt(values []Value, offset int64) (int, error) {
	length := int64(len(col.definitionLevels))
	if offset < 0 {
		return 0, errRowIndexOutOfBounds(offset, length)
	}
	if offset >= length {
		return 0, io.EOF
	}
	if length -= offset; length < int64(len(values)) {
		values = values[:length]
	}

	numNulls1 := int64(countLevelsNotEqual(col.definitionLevels[:offset], col.maxDefinitionLevel))
	numNulls2 := int64(countLevelsNotEqual(col.definitionLevels[offset:offset+length], col.maxDefinitionLevel))

	if numNulls2 < length {
		n, err := col.base.ReadValuesAt(values[:length-numNulls2], offset-numNulls1)
		if err != nil {
			return n, err
		}
	}

	if numNulls2 > 0 {
		columnIndex := ^int16(col.Column())
		i := numNulls2 - 1
		j := length - 1
		definitionLevels := col.definitionLevels[offset : offset+length]
		maxDefinitionLevel := col.maxDefinitionLevel

		for n := len(definitionLevels) - 1; n >= 0 && j > i; n-- {
			if definitionLevels[n] != maxDefinitionLevel {
				values[j] = Value{definitionLevel: definitionLevels[n], columnIndex: columnIndex}
			} else {
				values[j] = values[i]
				i--
			}
			j--
		}
	}

	return int(length), nil
}

// repeatedColumnBuffer is an implementation of the ColumnBuffer interface used
// as a wrapper to an underlying ColumnBuffer to manage the creation of
// repetition levels, definition levels, and map rows to the region of the
// underlying buffer that contains their sequence of values.
//
// Null values are not written to the underlying column; instead, the buffer
// tracks offsets of row values in the column, null row values are represented
// by the value -1 and a definition level less than the max.
//
// This column buffer type is used for all leaf columns that have a non-zero
// max repetition level, which may be because the column or one of its parent(s)
// are marked repeated.
type repeatedColumnBuffer struct {
	base               ColumnBuffer
	maxRepetitionLevel byte
	maxDefinitionLevel byte
	rows               []region
	repetitionLevels   []byte
	definitionLevels   []byte
	buffer             []Value
	reordering         *repeatedColumnBuffer
	nullOrdering       nullOrdering
}

// The region type maps the logical offset of rows within the repetition and
// definition levels, to the base offsets in the underlying column buffers
// where the non-null values have been written.
type region struct {
	offset     uint32
	baseOffset uint32
}

func sizeOfRegion(regions []region) int64 { return 8 * int64(len(regions)) }

func newRepeatedColumnBuffer(base ColumnBuffer, maxRepetitionLevel, maxDefinitionLevel byte, nullOrdering nullOrdering) *repeatedColumnBuffer {
	n := base.Cap()
	return &repeatedColumnBuffer{
		base:               base,
		maxRepetitionLevel: maxRepetitionLevel,
		maxDefinitionLevel: maxDefinitionLevel,
		rows:               make([]region, 0, n/8),
		repetitionLevels:   make([]byte, 0, n),
		definitionLevels:   make([]byte, 0, n),
		nullOrdering:       nullOrdering,
	}
}

func (col *repeatedColumnBuffer) Clone() ColumnBuffer {
	return &repeatedColumnBuffer{
		base:               col.base.Clone(),
		maxRepetitionLevel: col.maxRepetitionLevel,
		maxDefinitionLevel: col.maxDefinitionLevel,
		rows:               append([]region{}, col.rows...),
		repetitionLevels:   append([]byte{}, col.repetitionLevels...),
		definitionLevels:   append([]byte{}, col.definitionLevels...),
		nullOrdering:       col.nullOrdering,
	}
}

func (col *repeatedColumnBuffer) Type() Type {
	return col.base.Type()
}

func (col *repeatedColumnBuffer) NumValues() int64 {
	return int64(len(col.definitionLevels))
}

func (col *repeatedColumnBuffer) ColumnIndex() ColumnIndex {
	return columnIndexOfNullable(col.base, col.maxDefinitionLevel, col.definitionLevels)
}

func (col *repeatedColumnBuffer) OffsetIndex() OffsetIndex {
	return col.base.OffsetIndex()
}

func (col *repeatedColumnBuffer) BloomFilter() BloomFilter {
	return col.base.BloomFilter()
}

func (col *repeatedColumnBuffer) Dictionary() Dictionary {
	return col.base.Dictionary()
}

func (col *repeatedColumnBuffer) Column() int {
	return col.base.Column()
}

func (col *repeatedColumnBuffer) Pages() Pages {
	return onePage(col.Page())
}

func (col *repeatedColumnBuffer) Page() BufferedPage {
	if repeatedRowsHaveBeenReordered(col.rows) {
		if col.reordering == nil {
			col.reordering = col.Clone().(*repeatedColumnBuffer)
		}

		column := col.reordering
		column.Reset()
		maxNumValues := 0
		defer func() {
			clearValues(col.buffer[:maxNumValues])
		}()

		baseOffset := 0

		for _, row := range col.rows {
			rowOffset := int(row.offset)
			rowLength := repeatedRowLength(col.repetitionLevels[rowOffset:])
			numNulls := countLevelsNotEqual(col.definitionLevels[rowOffset:rowOffset+rowLength], col.maxDefinitionLevel)
			numValues := rowLength - numNulls

			if numValues > 0 {
				if numValues > cap(col.buffer) {
					col.buffer = make([]Value, numValues)
				} else {
					col.buffer = col.buffer[:numValues]
				}
				n, err := col.base.ReadValuesAt(col.buffer, int64(row.baseOffset))
				if err != nil && n < numValues {
					return newErrorPage(col.Type(), col.Column(), "reordering rows of repeated column: %w", err)
				}
				if _, err := column.base.WriteValues(col.buffer); err != nil {
					return newErrorPage(col.Type(), col.Column(), "reordering rows of repeated column: %w", err)
				}
				if numValues > maxNumValues {
					maxNumValues = numValues
				}
			}

			column.rows = append(column.rows, region{
				offset:     uint32(len(column.repetitionLevels)),
				baseOffset: uint32(baseOffset),
			})

			column.repetitionLevels = append(column.repetitionLevels, col.repetitionLevels[rowOffset:rowOffset+rowLength]...)
			column.definitionLevels = append(column.definitionLevels, col.definitionLevels[rowOffset:rowOffset+rowLength]...)
			baseOffset += numValues
		}

		col.swapReorderingBuffer(column)
	}

	return newRepeatedPage(
		col.base.Page(),
		col.maxRepetitionLevel,
		col.maxDefinitionLevel,
		col.repetitionLevels,
		col.definitionLevels,
	)
}

func (col *repeatedColumnBuffer) swapReorderingBuffer(buf *repeatedColumnBuffer) {
	col.base, buf.base = buf.base, col.base
	col.rows, buf.rows = buf.rows, col.rows
	col.repetitionLevels, buf.repetitionLevels = buf.repetitionLevels, col.repetitionLevels
	col.definitionLevels, buf.definitionLevels = buf.definitionLevels, col.definitionLevels
}

func (col *repeatedColumnBuffer) Reset() {
	col.base.Reset()
	col.rows = col.rows[:0]
	col.repetitionLevels = col.repetitionLevels[:0]
	col.definitionLevels = col.definitionLevels[:0]
}

func (col *repeatedColumnBuffer) Size() int64 {
	return sizeOfRegion(col.rows) + int64(len(col.repetitionLevels)) + int64(len(col.definitionLevels)) + col.base.Size()
}

func (col *repeatedColumnBuffer) Cap() int { return cap(col.rows) }

func (col *repeatedColumnBuffer) Len() int { return len(col.rows) }

func (col *repeatedColumnBuffer) Less(i, j int) bool {
	row1 := col.rows[i]
	row2 := col.rows[j]
	less := col.nullOrdering
	row1Length := repeatedRowLength(col.repetitionLevels[row1.offset:])
	row2Length := repeatedRowLength(col.repetitionLevels[row2.offset:])

	for k := 0; k < row1Length && k < row2Length; k++ {
		x := int(row1.offset) + k
		y := int(row2.offset) + k
		definitionLevel1 := col.definitionLevels[j+k]
		definitionLevel2 := col.definitionLevels[j+k]
		switch {
		case less(col.base, x, y, col.maxDefinitionLevel, definitionLevel1, definitionLevel2):
			return true
		case less(col.base, y, x, col.maxDefinitionLevel, definitionLevel2, definitionLevel1):
			return false
		}
	}

	return row1Length < row2Length
}

func (col *repeatedColumnBuffer) Swap(i, j int) {
	// Because the underlying column does not contain null values, and may hold
	// an arbitrary number of values per row, we cannot swap its values at
	// indexes i and j. We swap the row indexes only, then reorder the base
	// column buffer when its view is materialized into a page by creating a
	// copy and writing rows back to it following the order of rows in the
	// repeated column buffer.
	col.rows[i], col.rows[j] = col.rows[j], col.rows[i]
}

func (col *repeatedColumnBuffer) WriteValues(values []Value) (numValues int, err error) {
	// The values may belong to the last row that was written if they do not
	// start with a repetition level less than the column's maximum.
	var continuation Row
	if len(values) > 0 && values[0].repetitionLevel != 0 {
		continuation, values = splitRowValues(values)
	}

	if len(continuation) > 0 {
		for i, v := range continuation {
			if v.definitionLevel == col.maxDefinitionLevel {
				if _, err := col.base.WriteValues(continuation[i : i+1]); err != nil {
					return numValues, err
				}
			}
			col.repetitionLevels = append(col.repetitionLevels, v.repetitionLevel)
			col.definitionLevels = append(col.definitionLevels, v.definitionLevel)
			numValues++
		}
	}

	maxNumValues := 0
	defer func() {
		clearValues(col.buffer[:maxNumValues])
	}()

	var row []Value

	for len(values) > 0 {
		row, values = splitRowValues(values)
		if err := col.writeRow(row); err != nil {
			return numValues, err
		}
		numValues += len(row)
		if len(col.buffer) > maxNumValues {
			maxNumValues = len(col.buffer)
		}
	}

	return numValues, nil
}

func (col *repeatedColumnBuffer) writeRow(row []Value) error {
	col.buffer = col.buffer[:0]
	for _, v := range row {
		if v.definitionLevel == col.maxDefinitionLevel {
			col.buffer = append(col.buffer, v)
		}
	}

	baseOffset := col.base.NumValues()
	if len(col.buffer) > 0 {
		if _, err := col.base.WriteValues(col.buffer); err != nil {
			return err
		}
	}

	col.rows = append(col.rows, region{
		offset:     uint32(len(col.repetitionLevels)),
		baseOffset: uint32(baseOffset),
	})

	for _, v := range row {
		col.repetitionLevels = append(col.repetitionLevels, v.repetitionLevel)
		col.definitionLevels = append(col.definitionLevels, v.definitionLevel)
	}

	return nil
}

func (col *repeatedColumnBuffer) ReadValuesAt(values []Value, offset int64) (int, error) {
	// TODO:
	panic("NOT IMPLEMENTED")
}

func optionalRowsHaveBeenReordered(rows []int32) bool {
	i := int32(0)
	for _, row := range rows {
		if row < 0 {
			// Skip any row that is null.
			continue
		}

		// If rows have been reordered the indices are not increasing exactly
		// one by one.
		if row != i {
			return true
		}

		// Only increment the index if the row is not null.
		i++
	}
	return false
}

func repeatedRowsHaveBeenReordered(rows []region) bool {
	lastOffset := uint32(0)
	for _, row := range rows {
		if row.offset < lastOffset {
			return true
		}
		lastOffset = row.offset
	}
	return false
}

// =============================================================================
// The types below are in-memory implementations of the ColumnBuffer interface
// for each parquet type.
//
// These column buffers are created by calling NewColumnBuffer on parquet.Type
// instances; each parquet type manages to construct column buffers of the
// appropriate type, which ensures that we are packing as many values as we
// can in memory.
//
// See Type.NewColumnBuffer for details about how these types get created.
// =============================================================================

type booleanColumnBuffer struct{ booleanPage }

func newBooleanColumnBuffer(typ Type, columnIndex int16, bufferSize int) *booleanColumnBuffer {
	return &booleanColumnBuffer{
		booleanPage: booleanPage{
			typ:         typ,
			bits:        make([]byte, 0, bufferSize),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *booleanColumnBuffer) Clone() ColumnBuffer {
	return &booleanColumnBuffer{
		booleanPage: booleanPage{
			typ:         col.typ,
			bits:        append([]byte{}, col.bits...),
			offset:      col.offset,
			numValues:   col.numValues,
			columnIndex: col.columnIndex,
		},
	}
}

func (col *booleanColumnBuffer) ColumnIndex() ColumnIndex {
	return booleanColumnIndex{&col.booleanPage}
}

func (col *booleanColumnBuffer) OffsetIndex() OffsetIndex {
	return booleanOffsetIndex{&col.booleanPage}
}

func (col *booleanColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *booleanColumnBuffer) Dictionary() Dictionary { return nil }

func (col *booleanColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *booleanColumnBuffer) Page() BufferedPage { return &col.booleanPage }

func (col *booleanColumnBuffer) Reset() {
	col.bits = col.bits[:0]
	col.offset = 0
	col.numValues = 0
}

func (col *booleanColumnBuffer) Cap() int { return 8 * cap(col.bits) }

func (col *booleanColumnBuffer) Len() int { return int(col.numValues) }

func (col *booleanColumnBuffer) Less(i, j int) bool {
	a := col.valueAt(i)
	b := col.valueAt(j)
	return a != b && !a
}

func (col *booleanColumnBuffer) valueAt(i int) bool {
	j := uint32(i) / 8
	k := uint32(i) % 8
	return ((col.bits[j] >> k) & 1) != 0
}

func (col *booleanColumnBuffer) setValueAt(i int, v bool) {
	// `offset` is always zero in the page of a column buffer
	j := uint32(i) / 8
	k := uint32(i) % 8
	x := byte(0)
	if v {
		x = 1
	}
	col.bits[j] = (col.bits[j] & ^(1 << k)) | (x << k)
}

func (col *booleanColumnBuffer) Swap(i, j int) {
	a := col.valueAt(i)
	b := col.valueAt(j)
	col.setValueAt(i, b)
	col.setValueAt(j, a)
}

func (col *booleanColumnBuffer) WriteBooleans(values []bool) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	col.writeValues(rows, unsafe.Sizeof(false), 0)
	return len(values), nil
}

func (col *booleanColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *booleanColumnBuffer) writeValues(rows array, size, offset uintptr) {
	numBytes := bits.ByteCount(uint(col.numValues) + uint(rows.len))
	if cap(col.bits) < numBytes {
		col.bits = append(make([]byte, 0, 2*cap(col.bits)), col.bits...)
	}
	col.bits = col.bits[:numBytes]
	i := 0
	r := 8 - (int(col.numValues) % 8)

	if r <= rows.len {
		// First we attempt to write enough bits to align the number of values
		// in the column buffer on 8 bytes. After this step the next bit should
		// be written at the zero'th index of a byte of the buffer.
		var b byte
		for i < r {
			v := *(*byte)(rows.index(i, size, offset))
			b |= (v & 1) << uint(i)
			i++
		}
		x := uint(col.numValues) / 8
		y := uint(col.numValues) % 8
		col.bits[x] |= (b << y) | (col.bits[x] & ^(0xFF << y))
		col.numValues += int32(i)

		if n := rows.len - i; n >= 8 {
			// At this stage, we know that that we have at least 8 bits to write
			// and the bits will be aligned on the address of a byte in the
			// output buffer. We can work on 8 values per loop iteration,
			// packing them into a single byte and writing it to the output
			// buffer. This effectively reduces by 87.5% the number of memory
			// stores that the program needs to perform to generate the values.
			for j := i + (n/8)*8; i < j; i += 8 {
				b0 := *(*byte)(rows.index(i+0, size, offset))
				b1 := *(*byte)(rows.index(i+1, size, offset))
				b2 := *(*byte)(rows.index(i+2, size, offset))
				b3 := *(*byte)(rows.index(i+3, size, offset))
				b4 := *(*byte)(rows.index(i+4, size, offset))
				b5 := *(*byte)(rows.index(i+5, size, offset))
				b6 := *(*byte)(rows.index(i+6, size, offset))
				b7 := *(*byte)(rows.index(i+7, size, offset))

				col.bits[col.numValues/8] = (b0 & 1) |
					((b1 & 1) << 1) |
					((b2 & 1) << 2) |
					((b3 & 1) << 3) |
					((b4 & 1) << 4) |
					((b5 & 1) << 5) |
					((b6 & 1) << 6) |
					((b7 & 1) << 7)
				col.numValues += 8
			}
		}
	}

	for i < rows.len {
		x := uint(col.numValues) / 8
		y := uint(col.numValues) % 8
		b := *(*byte)(rows.index(i, size, offset))
		col.bits[x] = ((b & 1) << y) | (col.bits[x] & ^(1 << y))
		col.numValues++
		i++
	}

	col.bits = col.bits[:bits.ByteCount(uint(col.numValues))]
}

func (col *booleanColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(col.numValues))
	case i >= int(col.numValues):
		return 0, io.EOF
	default:
		for n < len(values) && i < int(col.numValues) {
			values[n] = makeValueBoolean(col.valueAt(i))
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type int32ColumnBuffer struct{ int32Page }

func newInt32ColumnBuffer(typ Type, columnIndex int16, bufferSize int) *int32ColumnBuffer {
	return &int32ColumnBuffer{
		int32Page: int32Page{
			typ:         typ,
			values:      make([]int32, 0, bufferSize/4),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *int32ColumnBuffer) Clone() ColumnBuffer {
	return &int32ColumnBuffer{
		int32Page: int32Page{
			typ:         col.typ,
			values:      append([]int32{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *int32ColumnBuffer) ColumnIndex() ColumnIndex { return int32ColumnIndex{&col.int32Page} }

func (col *int32ColumnBuffer) OffsetIndex() OffsetIndex { return int32OffsetIndex{&col.int32Page} }

func (col *int32ColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *int32ColumnBuffer) Dictionary() Dictionary { return nil }

func (col *int32ColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *int32ColumnBuffer) Page() BufferedPage { return &col.int32Page }

func (col *int32ColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *int32ColumnBuffer) Cap() int { return cap(col.values) }

func (col *int32ColumnBuffer) Len() int { return len(col.values) }

func (col *int32ColumnBuffer) Less(i, j int) bool { return col.values[i] < col.values[j] }

func (col *int32ColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *int32ColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 4) != 0 {
		return 0, fmt.Errorf("cannot write INT32 values from input of size %d", len(b))
	}
	col.values = append(col.values, bits.BytesToInt32(b)...)
	return len(b), nil
}

func (col *int32ColumnBuffer) WriteInt32s(values []int32) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *int32ColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *int32ColumnBuffer) writeValues(rows array, size, offset uintptr) {
	if n := len(col.values) + rows.len; n > cap(col.values) {
		col.values = append(make([]int32, 0, max(n, 2*cap(col.values))), col.values...)
	}

	n := len(col.values)
	col.values = col.values[:n+rows.len]

	values := col.values[n:]
	for i := range values {
		values[i] = *(*int32)(rows.index(i, size, offset))
	}
}

func (col *int32ColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueInt32(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type int64ColumnBuffer struct{ int64Page }

func newInt64ColumnBuffer(typ Type, columnIndex int16, bufferSize int) *int64ColumnBuffer {
	return &int64ColumnBuffer{
		int64Page: int64Page{
			typ:         typ,
			values:      make([]int64, 0, bufferSize/8),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *int64ColumnBuffer) Clone() ColumnBuffer {
	return &int64ColumnBuffer{
		int64Page: int64Page{
			typ:         col.typ,
			values:      append([]int64{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *int64ColumnBuffer) ColumnIndex() ColumnIndex { return int64ColumnIndex{&col.int64Page} }

func (col *int64ColumnBuffer) OffsetIndex() OffsetIndex { return int64OffsetIndex{&col.int64Page} }

func (col *int64ColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *int64ColumnBuffer) Dictionary() Dictionary { return nil }

func (col *int64ColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *int64ColumnBuffer) Page() BufferedPage { return &col.int64Page }

func (col *int64ColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *int64ColumnBuffer) Cap() int { return cap(col.values) }

func (col *int64ColumnBuffer) Len() int { return len(col.values) }

func (col *int64ColumnBuffer) Less(i, j int) bool { return col.values[i] < col.values[j] }

func (col *int64ColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *int64ColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 8) != 0 {
		return 0, fmt.Errorf("cannot write INT64 values from input of size %d", len(b))
	}
	col.values = append(col.values, bits.BytesToInt64(b)...)
	return len(b), nil
}

func (col *int64ColumnBuffer) WriteInt64s(values []int64) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *int64ColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *int64ColumnBuffer) writeValues(rows array, size, offset uintptr) {
	if n := len(col.values) + rows.len; n > cap(col.values) {
		col.values = append(make([]int64, 0, max(n, 2*cap(col.values))), col.values...)
	}

	n := len(col.values)
	col.values = col.values[:n+rows.len]

	values := col.values[n:]
	for i := range values {
		values[i] = *(*int64)(rows.index(i, size, offset))
	}
}

func (col *int64ColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueInt64(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type int96ColumnBuffer struct{ int96Page }

func newInt96ColumnBuffer(typ Type, columnIndex int16, bufferSize int) *int96ColumnBuffer {
	return &int96ColumnBuffer{
		int96Page: int96Page{
			typ:         typ,
			values:      make([]deprecated.Int96, 0, bufferSize/12),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *int96ColumnBuffer) Clone() ColumnBuffer {
	return &int96ColumnBuffer{
		int96Page: int96Page{
			typ:         col.typ,
			values:      append([]deprecated.Int96{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *int96ColumnBuffer) ColumnIndex() ColumnIndex { return int96ColumnIndex{&col.int96Page} }

func (col *int96ColumnBuffer) OffsetIndex() OffsetIndex { return int96OffsetIndex{&col.int96Page} }

func (col *int96ColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *int96ColumnBuffer) Dictionary() Dictionary { return nil }

func (col *int96ColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *int96ColumnBuffer) Page() BufferedPage { return &col.int96Page }

func (col *int96ColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *int96ColumnBuffer) Cap() int { return cap(col.values) }

func (col *int96ColumnBuffer) Len() int { return len(col.values) }

func (col *int96ColumnBuffer) Less(i, j int) bool { return col.values[i].Less(col.values[j]) }

func (col *int96ColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *int96ColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 12) != 0 {
		return 0, fmt.Errorf("cannot write INT96 values from input of size %d", len(b))
	}
	col.values = append(col.values, deprecated.BytesToInt96(b)...)
	return len(b), nil
}

func (col *int96ColumnBuffer) WriteInt96s(values []deprecated.Int96) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *int96ColumnBuffer) WriteValues(values []Value) (int, error) {
	for _, v := range values {
		col.values = append(col.values, v.Int96())
	}
	return len(values), nil
}

func (col *int96ColumnBuffer) writeValues(rows array, size, offset uintptr) {
	for i := 0; i < rows.len; i++ {
		p := rows.index(i, size, offset)
		col.values = append(col.values, *(*deprecated.Int96)(p))
	}
}

func (col *int96ColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueInt96(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type floatColumnBuffer struct{ floatPage }

func newFloatColumnBuffer(typ Type, columnIndex int16, bufferSize int) *floatColumnBuffer {
	return &floatColumnBuffer{
		floatPage: floatPage{
			typ:         typ,
			values:      make([]float32, 0, bufferSize/4),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *floatColumnBuffer) Clone() ColumnBuffer {
	return &floatColumnBuffer{
		floatPage: floatPage{
			typ:         col.typ,
			values:      append([]float32{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *floatColumnBuffer) ColumnIndex() ColumnIndex { return floatColumnIndex{&col.floatPage} }

func (col *floatColumnBuffer) OffsetIndex() OffsetIndex { return floatOffsetIndex{&col.floatPage} }

func (col *floatColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *floatColumnBuffer) Dictionary() Dictionary { return nil }

func (col *floatColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *floatColumnBuffer) Page() BufferedPage { return &col.floatPage }

func (col *floatColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *floatColumnBuffer) Cap() int { return cap(col.values) }

func (col *floatColumnBuffer) Len() int { return len(col.values) }

func (col *floatColumnBuffer) Less(i, j int) bool { return col.values[i] < col.values[j] }

func (col *floatColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *floatColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 4) != 0 {
		return 0, fmt.Errorf("cannot write FLOAT values from input of size %d", len(b))
	}
	col.values = append(col.values, bits.BytesToFloat32(b)...)
	return len(b), nil
}

func (col *floatColumnBuffer) WriteFloats(values []float32) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *floatColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *floatColumnBuffer) writeValues(rows array, size, offset uintptr) {
	if n := len(col.values) + rows.len; n > cap(col.values) {
		col.values = append(make([]float32, 0, max(n, 2*cap(col.values))), col.values...)
	}

	n := len(col.values)
	col.values = col.values[:n+rows.len]

	values := col.values[n:]
	for i := range values {
		values[i] = *(*float32)(rows.index(i, size, offset))
	}
}

func (col *floatColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueFloat(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type doubleColumnBuffer struct{ doublePage }

func newDoubleColumnBuffer(typ Type, columnIndex int16, bufferSize int) *doubleColumnBuffer {
	return &doubleColumnBuffer{
		doublePage: doublePage{
			typ:         typ,
			values:      make([]float64, 0, bufferSize/8),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *doubleColumnBuffer) Clone() ColumnBuffer {
	return &doubleColumnBuffer{
		doublePage: doublePage{
			typ:         col.typ,
			values:      append([]float64{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *doubleColumnBuffer) ColumnIndex() ColumnIndex { return doubleColumnIndex{&col.doublePage} }

func (col *doubleColumnBuffer) OffsetIndex() OffsetIndex { return doubleOffsetIndex{&col.doublePage} }

func (col *doubleColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *doubleColumnBuffer) Dictionary() Dictionary { return nil }

func (col *doubleColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *doubleColumnBuffer) Page() BufferedPage { return &col.doublePage }

func (col *doubleColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *doubleColumnBuffer) Cap() int { return cap(col.values) }

func (col *doubleColumnBuffer) Len() int { return len(col.values) }

func (col *doubleColumnBuffer) Less(i, j int) bool { return col.values[i] < col.values[j] }

func (col *doubleColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *doubleColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 8) != 0 {
		return 0, fmt.Errorf("cannot write DOUBLE values from input of size %d", len(b))
	}
	col.values = append(col.values, bits.BytesToFloat64(b)...)
	return len(b), nil
}

func (col *doubleColumnBuffer) WriteDoubles(values []float64) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *doubleColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *doubleColumnBuffer) writeValues(rows array, size, offset uintptr) {
	if n := len(col.values) + rows.len; n > cap(col.values) {
		col.values = append(make([]float64, 0, max(n, 2*cap(col.values))), col.values...)
	}

	n := len(col.values)
	col.values = col.values[:n+rows.len]

	values := col.values[n:]
	for i := range values {
		values[i] = *(*float64)(rows.index(i, size, offset))
	}
}

func (col *doubleColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueDouble(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type byteArrayColumnBuffer struct {
	byteArrayPage
	offsets []uint32
}

func newByteArrayColumnBuffer(typ Type, columnIndex int16, bufferSize int) *byteArrayColumnBuffer {
	return &byteArrayColumnBuffer{
		byteArrayPage: byteArrayPage{
			typ:         typ,
			values:      make([]byte, 0, bufferSize/2),
			columnIndex: ^columnIndex,
		},
		offsets: make([]uint32, 0, bufferSize/8),
	}
}

func (col *byteArrayColumnBuffer) cloneOffsets() []uint32 {
	offsets := make([]uint32, len(col.offsets))
	copy(offsets, col.offsets)
	return offsets
}

func (col *byteArrayColumnBuffer) Clone() ColumnBuffer {
	return &byteArrayColumnBuffer{
		byteArrayPage: byteArrayPage{
			typ:         col.typ,
			values:      col.cloneValues(),
			numValues:   col.numValues,
			columnIndex: col.columnIndex,
		},
		offsets: col.cloneOffsets(),
	}
}

func (col *byteArrayColumnBuffer) ColumnIndex() ColumnIndex {
	return byteArrayColumnIndex{&col.byteArrayPage}
}

func (col *byteArrayColumnBuffer) OffsetIndex() OffsetIndex {
	return byteArrayOffsetIndex{&col.byteArrayPage}
}

func (col *byteArrayColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *byteArrayColumnBuffer) Dictionary() Dictionary { return nil }

func (col *byteArrayColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *byteArrayColumnBuffer) Page() BufferedPage {
	if len(col.offsets) > 0 && bits.OrderOfUint32(col.offsets) < 1 { // unordered?
		values := make([]byte, 0, len(col.values)) // TODO: pool this buffer?

		for _, offset := range col.offsets {
			values = plain.AppendByteArray(values, col.valueAt(offset))
		}

		col.values = values
		col.offsets = col.offsets[:0]

		for i := 0; i < len(col.values); {
			n := plain.ByteArrayLength(col.values[i:])
			col.offsets = append(col.offsets, uint32(i))
			i += plain.ByteArrayLengthSize
			i += n
		}
	}
	return &col.byteArrayPage
}

func (col *byteArrayColumnBuffer) Reset() {
	col.values = col.values[:0]
	col.offsets = col.offsets[:0]
	col.numValues = 0
}

func (col *byteArrayColumnBuffer) Cap() int { return cap(col.offsets) }

func (col *byteArrayColumnBuffer) Len() int { return len(col.offsets) }

func (col *byteArrayColumnBuffer) Less(i, j int) bool {
	a := col.valueAt(col.offsets[i])
	b := col.valueAt(col.offsets[j])
	return bytes.Compare(a, b) < 0
}

func (col *byteArrayColumnBuffer) Swap(i, j int) {
	col.offsets[i], col.offsets[j] = col.offsets[j], col.offsets[i]
}

func (col *byteArrayColumnBuffer) Write(b []byte) (int, error) {
	_, n, err := col.writeByteArrays(b)
	return n, err
}

func (col *byteArrayColumnBuffer) WriteByteArrays(values []byte) (int, error) {
	n, _, err := col.writeByteArrays(values)
	return n, err
}

func (col *byteArrayColumnBuffer) writeByteArrays(values []byte) (count, bytes int, err error) {
	baseCount, baseBytes := len(col.offsets), len(col.values)

	err = plain.RangeByteArrays(values, func(value []byte) error {
		col.append(bits.BytesToString(value))
		return nil
	})

	return len(col.offsets) - baseCount, len(col.values) - baseBytes, err
}

func (col *byteArrayColumnBuffer) WriteValues(values []Value) (int, error) {
	rows := array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.ptr))
	return len(values), nil
}

func (col *byteArrayColumnBuffer) writeValues(rows array, size, offset uintptr) {
	for i := 0; i < rows.len; i++ {
		p := rows.index(i, size, offset)
		col.append(*(*string)(p))
	}
}

func (col *byteArrayColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.offsets)))
	case i >= len(col.offsets):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.offsets) {
			values[n] = makeValueBytes(ByteArray, col.valueAt(col.offsets[i]))
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

func (col *byteArrayColumnBuffer) append(value string) {
	col.offsets = append(col.offsets, uint32(len(col.values)))
	col.values = plain.AppendByteArrayString(col.values, value)
	col.numValues++
}

type fixedLenByteArrayColumnBuffer struct {
	fixedLenByteArrayPage
	tmp []byte
}

func newFixedLenByteArrayColumnBuffer(typ Type, columnIndex int16, bufferSize int) *fixedLenByteArrayColumnBuffer {
	size := typ.Length()
	return &fixedLenByteArrayColumnBuffer{
		fixedLenByteArrayPage: fixedLenByteArrayPage{
			typ:         typ,
			size:        size,
			data:        make([]byte, 0, bufferSize),
			columnIndex: ^columnIndex,
		},
		tmp: make([]byte, size),
	}
}

func (col *fixedLenByteArrayColumnBuffer) Clone() ColumnBuffer {
	return &fixedLenByteArrayColumnBuffer{
		fixedLenByteArrayPage: fixedLenByteArrayPage{
			typ:         col.typ,
			size:        col.size,
			data:        append([]byte{}, col.data...),
			columnIndex: col.columnIndex,
		},
		tmp: make([]byte, col.size),
	}
}

func (col *fixedLenByteArrayColumnBuffer) ColumnIndex() ColumnIndex {
	return fixedLenByteArrayColumnIndex{&col.fixedLenByteArrayPage}
}

func (col *fixedLenByteArrayColumnBuffer) OffsetIndex() OffsetIndex {
	return fixedLenByteArrayOffsetIndex{&col.fixedLenByteArrayPage}
}

func (col *fixedLenByteArrayColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *fixedLenByteArrayColumnBuffer) Dictionary() Dictionary { return nil }

func (col *fixedLenByteArrayColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *fixedLenByteArrayColumnBuffer) Page() BufferedPage { return &col.fixedLenByteArrayPage }

func (col *fixedLenByteArrayColumnBuffer) Reset() { col.data = col.data[:0] }

func (col *fixedLenByteArrayColumnBuffer) Cap() int { return cap(col.data) / col.size }

func (col *fixedLenByteArrayColumnBuffer) Len() int { return len(col.data) / col.size }

func (col *fixedLenByteArrayColumnBuffer) Less(i, j int) bool {
	return bytes.Compare(col.index(i), col.index(j)) < 0
}

func (col *fixedLenByteArrayColumnBuffer) Swap(i, j int) {
	t, u, v := col.tmp[:col.size], col.index(i), col.index(j)
	copy(t, u)
	copy(u, v)
	copy(v, t)
}

func (col *fixedLenByteArrayColumnBuffer) index(i int) []byte {
	j := (i + 0) * col.size
	k := (i + 1) * col.size
	return col.data[j:k:k]
}

func (col *fixedLenByteArrayColumnBuffer) Write(b []byte) (int, error) {
	n, err := col.WriteFixedLenByteArrays(b)
	return n * col.size, err
}

func (col *fixedLenByteArrayColumnBuffer) WriteFixedLenByteArrays(values []byte) (int, error) {
	d, m := len(values)/col.size, len(values)%col.size
	if m != 0 {
		return 0, fmt.Errorf("cannot write FIXED_LEN_BYTE_ARRAY values of size %d from input of size %d", col.size, len(values))
	}
	col.data = append(col.data, values...)
	return d, nil
}

func (col *fixedLenByteArrayColumnBuffer) WriteValues(values []Value) (int, error) {
	for _, v := range values {
		col.data = append(col.data, v.ByteArray()...)
	}
	return len(values), nil
}

func (col *fixedLenByteArrayColumnBuffer) writeValues(rows array, size, offset uintptr) {
	for i := 0; i < rows.len; i++ {
		p := rows.index(i, size, offset)
		col.data = append(col.data, unsafe.Slice((*byte)(p), col.size)...)
	}
}

func (col *fixedLenByteArrayColumnBuffer) writeValues128(rows array, size, offset uintptr) {
	c := cap(col.data)
	n := len(col.data) + (16 * rows.len)
	if c < n {
		col.data = append(make([]byte, 0, max(n, 2*c)), col.data...)
	}

	firstIndex := len(col.data)
	col.data = col.data[:len(col.data)+(16*rows.len)]

	data := unsafe.Pointer(&col.data[firstIndex])
	for i := 0; i < rows.len; i++ {
		p := rows.index(i, size, offset)
		*(*[16]byte)(unsafe.Add(data, i*16)) = *(*[16]byte)(p)
	}
}

func (col *fixedLenByteArrayColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset) * col.size
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.data)/col.size))
	case i >= len(col.data):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.data) {
			values[n] = makeValueBytes(FixedLenByteArray, col.data[i:i+col.size])
			values[n].columnIndex = col.columnIndex
			n++
			i += col.size
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type uint32ColumnBuffer struct{ uint32Page }

func newUint32ColumnBuffer(typ Type, columnIndex int16, bufferSize int) *uint32ColumnBuffer {
	return &uint32ColumnBuffer{
		uint32Page: uint32Page{
			typ:         typ,
			values:      make([]uint32, 0, bufferSize/4),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *uint32ColumnBuffer) Clone() ColumnBuffer {
	return &uint32ColumnBuffer{
		uint32Page: uint32Page{
			typ:         col.typ,
			values:      append([]uint32{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *uint32ColumnBuffer) ColumnIndex() ColumnIndex { return uint32ColumnIndex{&col.uint32Page} }

func (col *uint32ColumnBuffer) OffsetIndex() OffsetIndex { return uint32OffsetIndex{&col.uint32Page} }

func (col *uint32ColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *uint32ColumnBuffer) Dictionary() Dictionary { return nil }

func (col *uint32ColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *uint32ColumnBuffer) Page() BufferedPage { return &col.uint32Page }

func (col *uint32ColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *uint32ColumnBuffer) Cap() int { return cap(col.values) }

func (col *uint32ColumnBuffer) Len() int { return len(col.values) }

func (col *uint32ColumnBuffer) Less(i, j int) bool { return col.values[i] < col.values[j] }

func (col *uint32ColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *uint32ColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 4) != 0 {
		return 0, fmt.Errorf("cannot write INT32 values from input of size %d", len(b))
	}
	col.values = append(col.values, bits.BytesToUint32(b)...)
	return len(b), nil
}

func (col *uint32ColumnBuffer) WriteUint32s(values []uint32) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *uint32ColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *uint32ColumnBuffer) writeValues(rows array, size, offset uintptr) {
	if n := len(col.values) + rows.len; n > cap(col.values) {
		col.values = append(make([]uint32, 0, max(n, 2*cap(col.values))), col.values...)
	}

	n := len(col.values)
	col.values = col.values[:n+rows.len]

	values := col.values[n:]
	for i := range values {
		values[i] = *(*uint32)(rows.index(i, size, offset))
	}
}

func (col *uint32ColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueUint32(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

type uint64ColumnBuffer struct{ uint64Page }

func newUint64ColumnBuffer(typ Type, columnIndex int16, bufferSize int) *uint64ColumnBuffer {
	return &uint64ColumnBuffer{
		uint64Page: uint64Page{
			typ:         typ,
			values:      make([]uint64, 0, bufferSize/8),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *uint64ColumnBuffer) Clone() ColumnBuffer {
	return &uint64ColumnBuffer{
		uint64Page: uint64Page{
			typ:         col.typ,
			values:      append([]uint64{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *uint64ColumnBuffer) ColumnIndex() ColumnIndex { return uint64ColumnIndex{&col.uint64Page} }

func (col *uint64ColumnBuffer) OffsetIndex() OffsetIndex { return uint64OffsetIndex{&col.uint64Page} }

func (col *uint64ColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *uint64ColumnBuffer) Dictionary() Dictionary { return nil }

func (col *uint64ColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *uint64ColumnBuffer) Page() BufferedPage { return &col.uint64Page }

func (col *uint64ColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *uint64ColumnBuffer) Cap() int { return cap(col.values) }

func (col *uint64ColumnBuffer) Len() int { return len(col.values) }

func (col *uint64ColumnBuffer) Less(i, j int) bool { return col.values[i] < col.values[j] }

func (col *uint64ColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *uint64ColumnBuffer) Write(b []byte) (int, error) {
	if (len(b) % 8) != 0 {
		return 0, fmt.Errorf("cannot write INT64 values from input of size %d", len(b))
	}
	col.values = append(col.values, bits.BytesToUint64(b)...)
	return len(b), nil
}

func (col *uint64ColumnBuffer) WriteUint64s(values []uint64) (int, error) {
	col.values = append(col.values, values...)
	return len(values), nil
}

func (col *uint64ColumnBuffer) WriteValues(values []Value) (int, error) {
	var rows = array{
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&values)),
		len: len(values),
	}
	var value Value
	col.writeValues(rows, unsafe.Sizeof(value), unsafe.Offsetof(value.u64))
	return len(values), nil
}

func (col *uint64ColumnBuffer) writeValues(rows array, size, offset uintptr) {
	if n := len(col.values) + rows.len; n > cap(col.values) {
		col.values = append(make([]uint64, 0, max(n, 2*cap(col.values))), col.values...)
	}

	n := len(col.values)
	col.values = col.values[:n+rows.len]

	values := col.values[n:]
	for i := range values {
		values[i] = *(*uint64)(rows.index(i, size, offset))
	}
}

func (col *uint64ColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = makeValueUint64(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

var (
	_ sort.Interface = (ColumnBuffer)(nil)
	_ io.Writer      = (*byteArrayColumnBuffer)(nil)
	_ io.Writer      = (*fixedLenByteArrayColumnBuffer)(nil)
)
