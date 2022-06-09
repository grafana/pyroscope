//go:build go1.18

package parquet

import (
	"io"
	"reflect"
)

// GenericReader is similar to a Reader but uses a type parameter to define the
// Go type representing the schema of rows being read.
//
// See GenericWriter for details about the benefits over the classic Reader API.
type GenericReader[T any] struct {
	base Reader
	read readFunc[T]
}

// NewGenericReader is like NewReader but returns GenericReader[T] suited to write
// rows of Go type T.
//
// The type parameter T should be a map, struct, or any. Any other types will
// cause a panic at runtime. Type checking is a lot more effective when the
// generic parameter is a struct type, using map and interface types is somewhat
// similar to using a Writer.
//
// If the option list may explicitly declare a schema, it must be compatible
// with the schema generated from T.
func NewGenericReader[T any](input io.ReaderAt, options ...ReaderOption) *GenericReader[T] {
	c, err := NewReaderConfig(options...)
	if err != nil {
		panic(err)
	}

	t := typeOf[T]()
	if c.Schema == nil {
		c.Schema = schemaOf(dereference(t))
	}

	f, err := openFile(input)
	if err != nil {
		panic(err)
	}

	r := &GenericReader[T]{
		base: Reader{
			file: reader{
				schema:   c.Schema,
				rowGroup: fileRowGroupOf(f),
			},
		},
	}

	if !nodesAreEqual(c.Schema, f.schema) {
		r.base.file.rowGroup = convertRowGroupTo(r.base.file.rowGroup, c.Schema)
	}

	r.base.read.init(r.base.file.schema, r.base.file.rowGroup)
	r.read = readFuncOf[T](t, r.base.file.schema)
	return r
}

func NewGenericRowGroupReader[T any](rowGroup RowGroup, options ...ReaderOption) *GenericReader[T] {
	c, err := NewReaderConfig(options...)
	if err != nil {
		panic(err)
	}

	t := typeOf[T]()
	if c.Schema == nil {
		c.Schema = schemaOf(dereference(t))
	}

	r := &GenericReader[T]{
		base: Reader{
			file: reader{
				schema:   c.Schema,
				rowGroup: rowGroup,
			},
		},
	}

	if !nodesAreEqual(c.Schema, rowGroup.Schema()) {
		r.base.file.rowGroup = convertRowGroupTo(r.base.file.rowGroup, c.Schema)
	}

	r.base.read.init(r.base.file.schema, r.base.file.rowGroup)
	r.read = readFuncOf[T](t, r.base.file.schema)
	return r
}

func (r *GenericReader[T]) Reset() {
	r.base.Reset()
}

func (r *GenericReader[T]) Read(rows []T) (int, error) {
	return r.read(r, rows)
}

func (r *GenericReader[T]) ReadRows(rows []Row) (int, error) {
	return r.base.ReadRows(rows)
}

func (r *GenericReader[T]) Schema() *Schema {
	return r.base.Schema()
}

func (r *GenericReader[T]) NumRows() int64 {
	return r.base.NumRows()
}

func (r *GenericReader[T]) SeekToRow(rowIndex int64) error {
	return r.base.SeekToRow(rowIndex)
}

func (r *GenericReader[T]) Close() error {
	return r.base.Close()
}

func (r *GenericReader[T]) readRows(rows []T) (int, error) {
	if cap(r.base.rowbuf) < len(rows) {
		r.base.rowbuf = make([]Row, len(rows))
	} else {
		r.base.rowbuf = r.base.rowbuf[:len(rows)]
	}

	n, err := r.base.ReadRows(r.base.rowbuf)
	if n > 0 {
		schema := r.base.Schema()

		for i, row := range r.base.rowbuf[:n] {
			if err := schema.Reconstruct(&rows[i], row); err != nil {
				return i, err
			}
		}
	}
	return n, err
}

var (
	_ Rows                = (*GenericReader[any])(nil)
	_ RowReaderWithSchema = (*Reader)(nil)

	_ Rows                = (*GenericReader[struct{}])(nil)
	_ RowReaderWithSchema = (*GenericReader[struct{}])(nil)

	_ Rows                = (*GenericReader[map[struct{}]struct{}])(nil)
	_ RowReaderWithSchema = (*GenericReader[map[struct{}]struct{}])(nil)
)

type readFunc[T any] func(*GenericReader[T], []T) (int, error)

func readFuncOf[T any](t reflect.Type, schema *Schema) readFunc[T] {
	switch t.Kind() {
	case reflect.Interface, reflect.Map:
		return (*GenericReader[T]).readRows

	case reflect.Struct:
		return (*GenericReader[T]).readRows

	case reflect.Pointer:
		if e := t.Elem(); e.Kind() == reflect.Struct {
			return (*GenericReader[T]).readRows
		}
	}
	panic("cannot create reader for values of type " + t.String())
}
