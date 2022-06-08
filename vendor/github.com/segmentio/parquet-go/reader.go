package parquet

import (
	"errors"
	"fmt"
	"io"
	"reflect"
)

// A Reader reads Go values from parquet files.
//
// This example showcases a typical use of parquet readers:
//
//	reader := parquet.NewReader(file)
//	rows := []RowType{}
//	for {
//		row := RowType{}
//		err := reader.Read(&row)
//		if err != nil {
//			if err == io.EOF {
//				break
//			}
//			...
//		}
//		rows = append(rows, row)
//	}
//	if err := reader.Close(); err != nil {
//		...
//	}
//
//
type Reader struct {
	seen     reflect.Type
	file     reader
	read     reader
	rowIndex int64
	rowbuf   []Row
}

// NewReader constructs a parquet reader reading rows from the given
// io.ReaderAt.
//
// In order to read parquet rows, the io.ReaderAt must be converted to a
// parquet.File. If r is already a parquet.File it is used directly; otherwise,
// the io.ReaderAt value is expected to either have a `Size() int64` method or
// implement io.Seeker in order to determine its size.
//
// The function panics if the reader configuration is invalid. Programs that
// cannot guarantee the validity of the options passed to NewReader should
// construct the reader configuration independently prior to calling this
// function:
//
//	config, err := parquet.NewReaderConfig(options...)
//	if err != nil {
//		// handle the configuration error
//		...
//	} else {
//		// this call to create a reader is guaranteed not to panic
//		reader := parquet.NewReader(input, config)
//		...
//	}
//
func NewReader(input io.ReaderAt, options ...ReaderOption) *Reader {
	c, err := NewReaderConfig(options...)
	if err != nil {
		panic(err)
	}

	f, err := openFile(input)
	if err != nil {
		panic(err)
	}

	r := &Reader{
		file: reader{
			schema:   f.schema,
			rowGroup: fileRowGroupOf(f),
		},
	}

	if c.Schema != nil {
		r.file.schema = c.Schema
		r.file.rowGroup = convertRowGroupTo(r.file.rowGroup, c.Schema)
	}

	r.read.init(r.file.schema, r.file.rowGroup)
	return r
}

func openFile(input io.ReaderAt) (*File, error) {
	f, _ := input.(*File)
	if f != nil {
		return f, nil
	}
	n, err := sizeOf(input)
	if err != nil {
		return nil, err
	}
	return OpenFile(input, n)
}

func fileRowGroupOf(f *File) RowGroup {
	switch rowGroups := f.RowGroups(); len(rowGroups) {
	case 0:
		return newEmptyRowGroup(f.Schema())
	case 1:
		return rowGroups[0]
	default:
		// TODO: should we attempt to merge the row groups via MergeRowGroups
		// to preserve the global order of sorting columns within the file?
		return MultiRowGroup(rowGroups...)
	}
}

// NewRowGroupReader constructs a new Reader which reads rows from the RowGroup
// passed as argument.
func NewRowGroupReader(rowGroup RowGroup, options ...ReaderOption) *Reader {
	c, err := NewReaderConfig(options...)
	if err != nil {
		panic(err)
	}

	if c.Schema != nil {
		rowGroup = convertRowGroupTo(rowGroup, c.Schema)
	}

	r := &Reader{
		file: reader{
			schema:   rowGroup.Schema(),
			rowGroup: rowGroup,
		},
	}

	r.read.init(r.file.schema, r.file.rowGroup)
	return r
}

func convertRowGroupTo(rowGroup RowGroup, schema *Schema) RowGroup {
	if rowGroupSchema := rowGroup.Schema(); !nodesAreEqual(schema, rowGroupSchema) {
		conv, err := Convert(schema, rowGroupSchema)
		if err != nil {
			// TODO: this looks like something we should not be panicking on,
			// but the current NewReader API does not offer a mechanism to
			// report errors.
			panic(err)
		}
		rowGroup = ConvertRowGroup(rowGroup, conv)
	}
	return rowGroup
}

func sizeOf(r io.ReaderAt) (int64, error) {
	switch f := r.(type) {
	case interface{ Size() int64 }:
		return f.Size(), nil
	case io.Seeker:
		off, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}
		end, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		_, err = f.Seek(off, io.SeekStart)
		return end, err
	default:
		return 0, fmt.Errorf("cannot determine length of %T", r)
	}
}

// Reset repositions the reader at the beginning of the underlying parquet file.
func (r *Reader) Reset() {
	r.file.Reset()
	r.read.Reset()
	r.rowIndex = 0
	clearRows(r.rowbuf)
}

// Read reads the next row from r. The type of the row must match the schema
// of the underlying parquet file or an error will be returned.
//
// The method returns io.EOF when no more rows can be read from r.
func (r *Reader) Read(row interface{}) error {
	if rowType := dereference(reflect.TypeOf(row)); rowType.Kind() == reflect.Struct {
		if r.seen != rowType {
			if err := r.updateReadSchema(rowType); err != nil {
				return fmt.Errorf("cannot read parquet row into go value of type %T: %w", row, err)
			}
		}
	}

	if err := r.read.SeekToRow(r.rowIndex); err != nil {
		if errors.Is(err, io.ErrClosedPipe) {
			return io.EOF
		}
		return fmt.Errorf("seeking reader to row %d: %w", r.rowIndex, err)
	}

	if cap(r.rowbuf) == 0 {
		r.rowbuf = make([]Row, 1)
	} else {
		r.rowbuf = r.rowbuf[:1]
	}

	n, err := r.read.ReadRows(r.rowbuf[:])
	if n == 0 {
		return err
	}

	r.rowIndex++
	return r.read.schema.Reconstruct(row, r.rowbuf[0])
}

func (r *Reader) updateReadSchema(rowType reflect.Type) error {
	schema := schemaOf(rowType)

	if nodesAreEqual(schema, r.file.schema) {
		r.read.init(schema, r.file.rowGroup)
	} else {
		conv, err := Convert(schema, r.file.schema)
		if err != nil {
			return err
		}
		r.read.init(schema, ConvertRowGroup(r.file.rowGroup, conv))
	}

	r.seen = rowType
	return nil
}

// ReadRows reads the next rows from r into the given Row buffer.
//
// The returned values are laid out in the order expected by the
// parquet.(*Schema).Reconstruct method.
//
// The method returns io.EOF when no more rows can be read from r.
func (r *Reader) ReadRows(rows []Row) (int, error) {
	if err := r.file.SeekToRow(r.rowIndex); err != nil {
		return 0, err
	}
	n, err := r.file.ReadRows(rows)
	r.rowIndex += int64(n)
	return n, err
}

// Schema returns the schema of rows read by r.
func (r *Reader) Schema() *Schema { return r.file.schema }

// NumRows returns the number of rows that can be read from r.
func (r *Reader) NumRows() int64 { return r.file.rowGroup.NumRows() }

// SeekToRow positions r at the given row index.
func (r *Reader) SeekToRow(rowIndex int64) error {
	if err := r.file.SeekToRow(rowIndex); err != nil {
		return err
	}
	r.rowIndex = rowIndex
	return nil
}

// Close closes the reader, preventing more rows from being read.
func (r *Reader) Close() error {
	if err := r.read.Close(); err != nil {
		return err
	}
	if err := r.file.Close(); err != nil {
		return err
	}
	return nil
}

// reader is a subtype used in the implementation of Reader to support the two
// use cases of either reading rows calling the ReadRow method (where full rows
// are read from the underlying parquet file), or calling the Read method to
// read rows into Go values, potentially doing partial reads on a subset of the
// columns due to using a converted row group view.
type reader struct {
	schema   *Schema
	rowGroup RowGroup
	rows     Rows
	rowIndex int64
}

func (r *reader) init(schema *Schema, rowGroup RowGroup) {
	r.schema = schema
	r.rowGroup = rowGroup
	r.Reset()
}

func (r *reader) Reset() {
	r.rowIndex = 0

	if rows, ok := r.rows.(interface{ Reset() }); ok {
		// This optimization works for the common case where the underlying type
		// of the Rows instance is rowGroupRows, which should be true in most
		// cases since even external implementations of the RowGroup interface
		// can construct values of this type via the NewRowGroupRowReader
		// function.
		//
		// Foreign implementations of the Rows interface may also define a Reset
		// method in order to participate in this optimization.
		rows.Reset()
		return
	}

	if r.rows != nil {
		r.rows.Close()
		r.rows = nil
	}
}

func (r *reader) ReadRows(rows []Row) (int, error) {
	if r.rowGroup == nil {
		return 0, io.EOF
	}
	if r.rows == nil {
		r.rows = r.rowGroup.Rows()
		if r.rowIndex > 0 {
			if err := r.rows.SeekToRow(r.rowIndex); err != nil {
				return 0, err
			}
		}
	}
	n, err := r.rows.ReadRows(rows)
	r.rowIndex += int64(n)
	return n, err
}

func (r *reader) SeekToRow(rowIndex int64) error {
	if r.rowGroup == nil {
		return io.ErrClosedPipe
	}
	if rowIndex != r.rowIndex {
		if r.rows != nil {
			if err := r.rows.SeekToRow(rowIndex); err != nil {
				return err
			}
		}
		r.rowIndex = rowIndex
	}
	return nil
}

func (r *reader) Close() (err error) {
	r.rowGroup = nil
	if r.rows != nil {
		err = r.rows.Close()
	}
	return err
}

var (
	_ Rows                = (*Reader)(nil)
	_ RowReaderWithSchema = (*Reader)(nil)

	_ RowReader = (*reader)(nil)
	_ RowSeeker = (*reader)(nil)
)
