//go:build go1.18

package parquet

import (
	"reflect"
	"sort"
)

// GenericBuffer is similar to a Buffer but uses a type parameter to define the
// Go type representing the schema of rows in the buffer.
//
// See GenericWriter for details about the benefits over the classic Buffer API.
type GenericBuffer[T any] struct {
	base    Buffer
	write   bufferFunc[T]
	columns columnBufferWriter
}

// NewGenericBuffer is like NewBuffer but returns a GenericBuffer[T] suited to write
// rows of Go type T.
//
// The type parameter T should be a map, struct, or any. Any other types will
// cause a panic at runtime. Type checking is a lot more effective when the
// generic parameter is a struct type, using map and interface types is somewhat
// similar to using a Writer.
//
// If the option list may explicitly declare a schema, it must be compatible
// with the schema generated from T.
func NewGenericBuffer[T any](options ...RowGroupOption) *GenericBuffer[T] {
	config, err := NewRowGroupConfig(options...)
	if err != nil {
		panic(err)
	}

	t := typeOf[T]()
	if config.Schema == nil {
		config.Schema = schemaOf(dereference(t))
	}

	buf := &GenericBuffer[T]{
		base: Buffer{config: config},
	}
	buf.base.configure(config.Schema)
	buf.write = bufferFuncOf[T](t, config.Schema)
	buf.columns = columnBufferWriter{
		columns: buf.base.columns,
		values:  make([]Value, 0, defaultValueBufferSize),
	}
	return buf
}

func typeOf[T any]() reflect.Type {
	var v T
	return reflect.TypeOf(v)
}

type bufferFunc[T any] func(*GenericBuffer[T], []T) (int, error)

func bufferFuncOf[T any](t reflect.Type, schema *Schema) bufferFunc[T] {
	switch t.Kind() {
	case reflect.Interface, reflect.Map:
		return (*GenericBuffer[T]).writeRows

	case reflect.Struct:
		return makeBufferFunc[T](t, schema)

	case reflect.Pointer:
		if e := t.Elem(); e.Kind() == reflect.Struct {
			return makeBufferFunc[T](t, schema)
		}
	}
	panic("cannot create buffer for values of type " + t.String())
}

func makeBufferFunc[T any](t reflect.Type, schema *Schema) bufferFunc[T] {
	size := t.Size()
	writeRows := writeRowsFuncOf(t, schema, nil)
	return func(buf *GenericBuffer[T], rows []T) (n int, err error) {
		defer buf.columns.clear()
		err = writeRows(&buf.columns, makeArray(rows), size, 0, columnLevels{})
		if err == nil {
			n = len(rows)
		}
		return n, err
	}
}

func (buf *GenericBuffer[T]) Size() int64 {
	return buf.base.Size()
}

func (buf *GenericBuffer[T]) NumRows() int64 {
	return buf.base.NumRows()
}

func (buf *GenericBuffer[T]) ColumnChunks() []ColumnChunk {
	return buf.base.ColumnChunks()
}

func (buf *GenericBuffer[T]) ColumnBuffers() []ColumnBuffer {
	return buf.base.ColumnBuffers()
}

func (buf *GenericBuffer[T]) SortingColumns() []SortingColumn {
	return buf.base.SortingColumns()
}

func (buf *GenericBuffer[T]) Len() int {
	return buf.base.Len()
}

func (buf *GenericBuffer[T]) Less(i, j int) bool {
	return buf.base.Less(i, j)
}

func (buf *GenericBuffer[T]) Swap(i, j int) {
	buf.base.Swap(i, j)
}

func (buf *GenericBuffer[T]) Reset() {
	buf.base.Reset()
}

func (buf *GenericBuffer[T]) Write(rows []T) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	return buf.write(buf, rows)
}

func (buf *GenericBuffer[T]) WriteRows(rows []Row) (int, error) {
	return buf.base.WriteRows(rows)
}

func (buf *GenericBuffer[T]) WriteRowGroup(rowGroup RowGroup) (int64, error) {
	return buf.base.WriteRowGroup(rowGroup)
}

func (buf *GenericBuffer[T]) Rows() Rows {
	return buf.base.Rows()
}

func (buf *GenericBuffer[T]) Schema() *Schema {
	return buf.base.Schema()
}

func (buf *GenericBuffer[T]) writeRows(rows []T) (int, error) {
	if cap(buf.base.rowbuf) < len(rows) {
		buf.base.rowbuf = make([]Row, len(rows))
	} else {
		buf.base.rowbuf = buf.base.rowbuf[:len(rows)]
	}
	defer clearRows(buf.base.rowbuf)

	schema := buf.base.Schema()
	for i := range rows {
		buf.base.rowbuf[i] = schema.Deconstruct(buf.base.rowbuf[i], &rows[i])
	}

	return buf.base.WriteRows(buf.base.rowbuf)
}

var (
	_ RowGroup       = (*GenericBuffer[any])(nil)
	_ RowGroupWriter = (*GenericBuffer[any])(nil)
	_ sort.Interface = (*GenericBuffer[any])(nil)

	_ RowGroup       = (*GenericBuffer[struct{}])(nil)
	_ RowGroupWriter = (*GenericBuffer[struct{}])(nil)
	_ sort.Interface = (*GenericBuffer[struct{}])(nil)

	_ RowGroup       = (*GenericBuffer[map[struct{}]struct{}])(nil)
	_ RowGroupWriter = (*GenericBuffer[map[struct{}]struct{}])(nil)
	_ sort.Interface = (*GenericBuffer[map[struct{}]struct{}])(nil)
)
