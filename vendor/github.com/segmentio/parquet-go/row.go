package parquet

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
)

const (
	defaultRowBufferSize = 20
)

// Row represents a parquet row as a slice of values.
//
// Each value should embed a column index, repetition level, and definition
// level allowing the program to determine how to reconstruct the original
// object from the row. Repeated values share the same column index, their
// relative position of repeated values is represented by their relative
// position in the row.
type Row []Value

// Equal returns true if row and other contain the same sequence of values.
func (row Row) Equal(other Row) bool {
	if len(row) != len(other) {
		return false
	}
	for i := range row {
		if !Equal(row[i], other[i]) {
			return false
		}
		if row[i].repetitionLevel != other[i].repetitionLevel {
			return false
		}
		if row[i].definitionLevel != other[i].definitionLevel {
			return false
		}
		if row[i].columnIndex != other[i].columnIndex {
			return false
		}
	}
	return true
}

func (row Row) startsWith(columnIndex int16) bool {
	return len(row) > 0 && row[0].Column() == int(columnIndex)
}

// RowSeeker is an interface implemented by readers of parquet rows which can be
// positioned at a specific row index.
type RowSeeker interface {
	// Positions the stream on the given row index.
	//
	// Some implementations of the interface may only allow seeking forward.
	//
	// The method returns io.ErrClosedPipe if the stream had already been closed.
	SeekToRow(int64) error
}

// RowReader reads a sequence of parquet rows.
type RowReader interface {
	// Read rows from the reader, returning the number of rows read into the
	// buffer, and any error that occurred.
	//
	// When all rows have been read, the reader returns io.EOF to indicate the
	// end of the sequence. It is valid for the reader to return both a non-zero
	// number of rows and a non-nil error (including io.EOF).
	//
	// The buffer of rows passed as argument will be used to store values of
	// each row read from the reader. If the rows are not nil, the backing array
	// of the slices will be used as an optimization to avoid re-allocating new
	// arrays.
	ReadRows([]Row) (int, error)
}

// RowReaderFrom reads parquet rows from reader.
type RowReaderFrom interface {
	ReadRowsFrom(RowReader) (int64, error)
}

// RowReaderWithSchema is an extension of the RowReader interface which
// advertises the schema of rows returned by ReadRow calls.
type RowReaderWithSchema interface {
	RowReader
	Schema() *Schema
}

// RowReadSeeker is an interface implemented by row readers which support
// seeking to arbitrary row positions.
type RowReadSeeker interface {
	RowReader
	RowSeeker
}

// RowWriter writes parquet rows to an underlying medium.
type RowWriter interface {
	// Writes rows to the writer, returning the number of rows written and any
	// error that occurred.
	//
	// Because columnar operations operate on independent columns of values,
	// writes of rows may not be atomic operations, and could result in some
	// rows being partially written. The method returns the number of rows that
	// were successfully written, but if an error occurs, values of the row(s)
	// that failed to be written may have been partially committed to their
	// columns. For that reason, applications should consider a write error as
	// fatal and assume that they need to discard the state, they cannot retry
	// the write nor recover the underlying file.
	WriteRows([]Row) (int, error)
}

// RowWriterTo writes parquet rows to a writer.
type RowWriterTo interface {
	WriteRowsTo(RowWriter) (int64, error)
}

// RowWriterWithSchema is an extension of the RowWriter interface which
// advertises the schema of rows expected to be passed to WriteRow calls.
type RowWriterWithSchema interface {
	RowWriter
	Schema() *Schema
}

type forwardRowSeeker struct {
	rows  RowReader
	seek  int64
	index int64
}

func (r *forwardRowSeeker) ReadRows(rows []Row) (int, error) {
	for {
		n, err := r.rows.ReadRows(rows)

		if n > 0 && r.index < r.seek {
			skip := r.seek - r.index
			r.index += int64(n)
			if skip >= int64(n) {
				continue
			}

			for i, j := 0, int(skip); j < n; i++ {
				rows[i] = append(rows[i][:0], rows[j]...)
			}

			n -= int(skip)
		}

		return n, err
	}
}

func (r *forwardRowSeeker) SeekToRow(rowIndex int64) error {
	if rowIndex >= r.index {
		r.seek = rowIndex
		return nil
	}
	return fmt.Errorf("SeekToRow: %T does not implement parquet.RowSeeker: cannot seek backward from row %d to %d", r.rows, r.index, rowIndex)
}

// CopyRows copies rows from src to dst.
//
// The underlying types of src and dst are tested to determine if they expose
// information about the schema of rows that are read and expected to be
// written. If the schema information are available but do not match, the
// function will attempt to automatically convert the rows from the source
// schema to the destination.
//
// As an optimization, the src argument may implement RowWriterTo to bypass
// the default row copy logic and provide its own. The dst argument may also
// implement RowReaderFrom for the same purpose.
//
// The function returns the number of rows written, or any error encountered
// other than io.EOF.
func CopyRows(dst RowWriter, src RowReader) (int64, error) {
	return copyRows(dst, src, nil)
}

func copyRows(dst RowWriter, src RowReader, buf []Row) (written int64, err error) {
	targetSchema := targetSchemaOf(dst)
	sourceSchema := sourceSchemaOf(src)

	if targetSchema != nil && sourceSchema != nil {
		if !nodesAreEqual(targetSchema, sourceSchema) {
			conv, err := Convert(targetSchema, sourceSchema)
			if err != nil {
				return 0, err
			}
			// The conversion effectively disables a potential optimization
			// if the source reader implemented RowWriterTo. It is a trade off
			// we are making to optimize for safety rather than performance.
			//
			// Entering this code path should not be the common case tho, it is
			// most often used when parquet schemas are evolving, but we expect
			// that the majority of files of an application to be sharing a
			// common schema.
			src = ConvertRowReader(src, conv)
		}
	}

	if wt, ok := src.(RowWriterTo); ok {
		return wt.WriteRowsTo(dst)
	}

	if rf, ok := dst.(RowReaderFrom); ok {
		return rf.ReadRowsFrom(src)
	}

	if len(buf) == 0 {
		buf = make([]Row, defaultRowBufferSize)
	}

	defer clearRows(buf)

	for {
		rn, err := src.ReadRows(buf)

		if rn > 0 {
			wn, err := dst.WriteRows(buf[:rn])
			if err != nil {
				return written, err
			}

			written += int64(wn)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			return written, err
		}

		if rn == 0 {
			return written, io.ErrNoProgress
		}
	}
}

func clearRows(rows []Row) {
	for i, values := range rows {
		clearValues(values)
		rows[i] = values[:0]
	}
}

func sourceSchemaOf(r RowReader) *Schema {
	if rrs, ok := r.(RowReaderWithSchema); ok {
		return rrs.Schema()
	}
	return nil
}

func targetSchemaOf(w RowWriter) *Schema {
	if rws, ok := w.(RowWriterWithSchema); ok {
		return rws.Schema()
	}
	return nil
}

func errRowIndexOutOfBounds(rowIndex, rowCount int64) error {
	return fmt.Errorf("row index out of bounds: %d/%d", rowIndex, rowCount)
}

func hasRepeatedRowValues(values []Value) bool {
	for _, v := range values {
		if v.repetitionLevel != 0 {
			return true
		}
	}
	return false
}

// repeatedRowLength gives the length of the repeated row starting at the
// beginning of the repetitionLevels slice.
func repeatedRowLength(repetitionLevels []byte) int {
	// If a repetition level exists, at least one value is required to represent
	// the column.
	if len(repetitionLevels) > 0 {
		// The subsequent levels will represent the start of a new record when
		// they go back to zero.
		if i := bytes.IndexByte(repetitionLevels[1:], 0); i >= 0 {
			return i + 1
		}
	}
	return len(repetitionLevels)
}

func countRowsOf(values []Value) (numRows int) {
	if !hasRepeatedRowValues(values) {
		return len(values) // Faster path when there are no repeated values.
	}
	if len(values) > 0 {
		// The values may have not been at the start of a repeated row,
		// it could be the continuation of a repeated row. Skip until we
		// find the beginning of a row before starting to count how many
		// rows there are.
		if values[0].repetitionLevel != 0 {
			_, values = splitRowValues(values)
		}
		for len(values) > 0 {
			numRows++
			_, values = splitRowValues(values)
		}
	}
	return numRows
}

func limitRowValues(values []Value, rowCount int) []Value {
	if !hasRepeatedRowValues(values) {
		if len(values) > rowCount {
			values = values[:rowCount]
		}
	} else {
		var row Row
		var limit int
		for len(values) > 0 {
			row, values = splitRowValues(values)
			limit += len(row)
		}
		values = values[:limit]
	}
	return values
}

func splitRowValues(values []Value) (head, tail []Value) {
	for i, v := range values {
		if v.repetitionLevel == 0 {
			return values[:i+1], values[i+1:]
		}
	}
	return values, nil
}

// =============================================================================
// Functions returning closures are marked with "go:noinline" below to prevent
// losing naming information of the closure in stack traces.
//
// Because some of the functions are very short (simply return a closure), the
// compiler inlines when at their call site, which result in the closure being
// named something like parquet.deconstructFuncOf.func2 instead of the original
// parquet.deconstructFuncOfLeaf.func1; the latter being much more meaningful
// when reading CPU or memory profiles.
// =============================================================================

type levels struct {
	repetitionDepth byte
	repetitionLevel byte
	definitionLevel byte
}

type deconstructFunc func(Row, levels, reflect.Value) Row

func deconstructFuncOf(columnIndex int16, node Node) (int16, deconstructFunc) {
	switch {
	case node.Optional():
		return deconstructFuncOfOptional(columnIndex, node)
	case node.Repeated():
		return deconstructFuncOfRepeated(columnIndex, node)
	case isList(node):
		return deconstructFuncOfList(columnIndex, node)
	case isMap(node):
		return deconstructFuncOfMap(columnIndex, node)
	default:
		return deconstructFuncOfRequired(columnIndex, node)
	}
}

//go:noinline
func deconstructFuncOfOptional(columnIndex int16, node Node) (int16, deconstructFunc) {
	columnIndex, deconstruct := deconstructFuncOf(columnIndex, Required(node))
	return columnIndex, func(row Row, levels levels, value reflect.Value) Row {
		if value.IsValid() {
			if value.IsZero() {
				value = reflect.Value{}
			} else {
				if value.Kind() == reflect.Ptr {
					value = value.Elem()
				}
				levels.definitionLevel++
			}
		}
		return deconstruct(row, levels, value)
	}
}

//go:noinline
func deconstructFuncOfRepeated(columnIndex int16, node Node) (int16, deconstructFunc) {
	columnIndex, deconstruct := deconstructFuncOf(columnIndex, Required(node))
	return columnIndex, func(row Row, levels levels, value reflect.Value) Row {
		if !value.IsValid() || value.Len() == 0 {
			return deconstruct(row, levels, reflect.Value{})
		}

		levels.repetitionDepth++
		levels.definitionLevel++

		for i, n := 0, value.Len(); i < n; i++ {
			row = deconstruct(row, levels, value.Index(i))
			levels.repetitionLevel = levels.repetitionDepth
		}

		return row
	}
}

func deconstructFuncOfRequired(columnIndex int16, node Node) (int16, deconstructFunc) {
	switch {
	case node.Leaf():
		return deconstructFuncOfLeaf(columnIndex, node)
	default:
		return deconstructFuncOfGroup(columnIndex, node)
	}
}

func deconstructFuncOfList(columnIndex int16, node Node) (int16, deconstructFunc) {
	return deconstructFuncOf(columnIndex, Repeated(listElementOf(node)))
}

//go:noinline
func deconstructFuncOfMap(columnIndex int16, node Node) (int16, deconstructFunc) {
	keyValue := mapKeyValueOf(node)
	keyValueType := keyValue.GoType()
	keyValueElem := keyValueType.Elem()
	keyType := keyValueElem.Field(0).Type
	valueType := keyValueElem.Field(1).Type
	columnIndex, deconstruct := deconstructFuncOf(columnIndex, schemaOf(keyValueElem))
	return columnIndex, func(row Row, levels levels, mapValue reflect.Value) Row {
		if !mapValue.IsValid() || mapValue.Len() == 0 {
			return deconstruct(row, levels, reflect.Value{})
		}

		levels.repetitionDepth++
		levels.definitionLevel++

		elem := reflect.New(keyValueElem).Elem()
		k := elem.Field(0)
		v := elem.Field(1)

		for _, key := range mapValue.MapKeys() {
			k.Set(key.Convert(keyType))
			v.Set(mapValue.MapIndex(key).Convert(valueType))
			row = deconstruct(row, levels, elem)
			levels.repetitionLevel = levels.repetitionDepth
		}

		return row
	}
}

//go:noinline
func deconstructFuncOfGroup(columnIndex int16, node Node) (int16, deconstructFunc) {
	fields := node.Fields()
	funcs := make([]deconstructFunc, len(fields))
	for i, field := range fields {
		columnIndex, funcs[i] = deconstructFuncOf(columnIndex, field)
	}
	return columnIndex, func(row Row, levels levels, value reflect.Value) Row {
		if value.IsValid() {
			for i, f := range funcs {
				row = f(row, levels, fields[i].Value(value))
			}
		} else {
			for _, f := range funcs {
				row = f(row, levels, value)
			}
		}
		return row
	}
}

//go:noinline
func deconstructFuncOfLeaf(columnIndex int16, node Node) (int16, deconstructFunc) {
	if columnIndex > MaxColumnIndex {
		panic("row cannot be deconstructed because it has more than 127 columns")
	}
	kind := node.Type().Kind()
	valueColumnIndex := ^columnIndex
	return columnIndex + 1, func(row Row, levels levels, value reflect.Value) Row {
		v := Value{}

		if value.IsValid() {
			v = makeValue(kind, value)
		}

		v.repetitionLevel = levels.repetitionLevel
		v.definitionLevel = levels.definitionLevel
		v.columnIndex = valueColumnIndex
		return append(row, v)
	}
}

type reconstructFunc func(reflect.Value, levels, Row) (Row, error)

func reconstructFuncOf(columnIndex int16, node Node) (int16, reconstructFunc) {
	switch {
	case node.Optional():
		return reconstructFuncOfOptional(columnIndex, node)
	case node.Repeated():
		return reconstructFuncOfRepeated(columnIndex, node)
	case isList(node):
		return reconstructFuncOfList(columnIndex, node)
	case isMap(node):
		return reconstructFuncOfMap(columnIndex, node)
	default:
		return reconstructFuncOfRequired(columnIndex, node)
	}
}

//go:noinline
func reconstructFuncOfOptional(columnIndex int16, node Node) (int16, reconstructFunc) {
	nextColumnIndex, reconstruct := reconstructFuncOf(columnIndex, Required(node))
	rowLength := nextColumnIndex - columnIndex
	return nextColumnIndex, func(value reflect.Value, levels levels, row Row) (Row, error) {
		if !row.startsWith(columnIndex) {
			return row, fmt.Errorf("row is missing optional column %d", columnIndex)
		}
		if len(row) < int(rowLength) {
			return row, fmt.Errorf("expected optional column %d to have at least %d values but got %d", columnIndex, rowLength, len(row))
		}

		levels.definitionLevel++

		if row[0].definitionLevel < levels.definitionLevel {
			value.Set(reflect.Zero(value.Type()))
			return row[rowLength:], nil
		}

		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			value = value.Elem()
		}

		return reconstruct(value, levels, row)
	}
}

//go:noinline
func reconstructFuncOfRepeated(columnIndex int16, node Node) (int16, reconstructFunc) {
	nextColumnIndex, reconstruct := reconstructFuncOf(columnIndex, Required(node))
	rowLength := nextColumnIndex - columnIndex
	return nextColumnIndex, func(value reflect.Value, lvls levels, row Row) (Row, error) {
		t := value.Type()
		c := value.Cap()
		n := 0
		if c > 0 {
			value.Set(value.Slice(0, c))
		} else {
			c = 10
			value.Set(reflect.MakeSlice(t, c, c))
		}

		defer func() {
			value.Set(value.Slice(0, n))
		}()

		return reconstructRepeated(columnIndex, rowLength, lvls, row, func(levels levels, row Row) (Row, error) {
			if n == c {
				c *= 2
				newValue := reflect.MakeSlice(t, c, c)
				reflect.Copy(newValue, value)
				value.Set(newValue)
			}
			row, err := reconstruct(value.Index(n), levels, row)
			n++
			return row, err
		})
	}
}

func reconstructRepeated(columnIndex, rowLength int16, levels levels, row Row, do func(levels, Row) (Row, error)) (Row, error) {
	if !row.startsWith(columnIndex) {
		return row, fmt.Errorf("row is missing repeated column %d: %+v", columnIndex, row)
	}
	if len(row) < int(rowLength) {
		return row, fmt.Errorf("expected repeated column %d to have at least %d values but got %d", columnIndex, rowLength, len(row))
	}

	levels.repetitionDepth++
	levels.definitionLevel++

	if row[0].definitionLevel < levels.definitionLevel {
		return row[rowLength:], nil
	}

	var err error
	for row.startsWith(columnIndex) && row[0].repetitionLevel == levels.repetitionLevel {
		if row, err = do(levels, row); err != nil {
			break
		}
		levels.repetitionLevel = levels.repetitionDepth
	}
	return row, err
}

func reconstructFuncOfRequired(columnIndex int16, node Node) (int16, reconstructFunc) {
	switch {
	case node.Leaf():
		return reconstructFuncOfLeaf(columnIndex, node)
	default:
		return reconstructFuncOfGroup(columnIndex, node)
	}
}

func reconstructFuncOfList(columnIndex int16, node Node) (int16, reconstructFunc) {
	return reconstructFuncOf(columnIndex, Repeated(listElementOf(node)))
}

//go:noinline
func reconstructFuncOfMap(columnIndex int16, node Node) (int16, reconstructFunc) {
	keyValue := mapKeyValueOf(node)
	keyValueType := keyValue.GoType()
	keyValueElem := keyValueType.Elem()
	keyValueZero := reflect.Zero(keyValueElem)
	nextColumnIndex, reconstruct := reconstructFuncOf(columnIndex, schemaOf(keyValueElem))
	rowLength := nextColumnIndex - columnIndex
	return nextColumnIndex, func(mapValue reflect.Value, lvls levels, row Row) (Row, error) {
		t := mapValue.Type()
		k := t.Key()
		v := t.Elem()

		if mapValue.IsNil() {
			mapValue.Set(reflect.MakeMap(t))
		}

		elem := reflect.New(keyValueElem).Elem()
		return reconstructRepeated(columnIndex, rowLength, lvls, row, func(levels levels, row Row) (Row, error) {
			row, err := reconstruct(elem, levels, row)
			if err == nil {
				mapValue.SetMapIndex(elem.Field(0).Convert(k), elem.Field(1).Convert(v))
				elem.Set(keyValueZero)
			}
			return row, err
		})
	}
}

//go:noinline
func reconstructFuncOfGroup(columnIndex int16, node Node) (int16, reconstructFunc) {
	fields := node.Fields()
	funcs := make([]reconstructFunc, len(fields))
	columnIndexes := make([]int16, len(fields))

	for i, field := range fields {
		columnIndex, funcs[i] = reconstructFuncOf(columnIndex, field)
		columnIndexes[i] = columnIndex
	}

	return columnIndex, func(value reflect.Value, levels levels, row Row) (Row, error) {
		var err error

		for i, f := range funcs {
			if row, err = f(fields[i].Value(value), levels, row); err != nil {
				err = fmt.Errorf("%s → %w", fields[i].Name(), err)
				break
			}
		}

		return row, err
	}
}

//go:noinline
func reconstructFuncOfLeaf(columnIndex int16, node Node) (int16, reconstructFunc) {
	return columnIndex + 1, func(value reflect.Value, _ levels, row Row) (Row, error) {
		if !row.startsWith(columnIndex) {
			return row, fmt.Errorf("no values found in parquet row for column %d", columnIndex)
		}
		return row[1:], assignValue(value, row[0])
	}
}
