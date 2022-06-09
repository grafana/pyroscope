package pqarrow

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/segmentio/parquet-go"

	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/polarsignals/arcticdb/query/logicalplan"
)

// ParquetRowGroupToArrowRecord converts a parquet row group to an arrow record.
func ParquetRowGroupToArrowRecord(
	ctx context.Context,
	pool memory.Allocator,
	rg parquet.RowGroup,
	projections []logicalplan.ColumnMatcher,
	filterExpr logicalplan.Expr,
	distinctColumns []logicalplan.ColumnMatcher,
) (arrow.Record, error) {
	switch rg.(type) {
	case *dynparquet.MergedRowGroup:
		return rowBasedParquetRowGroupToArrowRecord(ctx, pool, rg)
	default:
		return contiguousParquetRowGroupToArrowRecord(
			ctx,
			pool,
			rg,
			projections,
			filterExpr,
			distinctColumns,
		)
	}
}

// rowBasedParquetRowGroupToArrowRecord converts a parquet row group to an arrow record row by row.
func rowBasedParquetRowGroupToArrowRecord(
	ctx context.Context,
	pool memory.Allocator,
	rg parquet.RowGroup,
) (arrow.Record, error) {
	s := rg.Schema()

	parquetFields := s.Fields()
	fields := make([]arrow.Field, 0, len(parquetFields))
	newWriterFuncs := make([]func(array.Builder, int) valueWriter, 0, len(parquetFields))
	for _, parquetField := range parquetFields {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			name := parquetField.Name()
			node := dynparquet.FieldByName(s, name)
			typ, newValueWriter, err := parquetNodeToType(node)
			if err != nil {
				return nil, err
			}
			nullable := false
			if node.Optional() {
				nullable = true
			}

			if node.Repeated() {
				typ = arrow.ListOf(typ)
				newValueWriter = newListValueWriter(newValueWriter)
			}
			newWriterFuncs = append(newWriterFuncs, newValueWriter)

			fields = append(fields, arrow.Field{
				Name:     name,
				Type:     typ,
				Nullable: nullable,
			})
		}
	}

	writers := make([]valueWriter, len(parquetFields))
	b := array.NewRecordBuilder(pool, arrow.NewSchema(fields, nil))
	for i, column := range b.Fields() {
		writers[i] = newWriterFuncs[i](column, 0)
	}

	rows := rg.Rows()
	defer rows.Close()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rowBuf := make([]parquet.Row, 64) // Random guess.
		n, err := rows.ReadRows(rowBuf)
		if err == io.EOF && n == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read row: %w", err)
		}
		rowBuf = rowBuf[:n]

		for i, writer := range writers {
			for _, row := range rowBuf {
				values := dynparquet.ValuesForIndex(row, i)
				writer.Write(values)
			}
		}
		if err == io.EOF {
			break
		}
	}

	return b.NewRecord(), nil
}

// contiguousParquetRowGroupToArrowRecord converts a parquet row group to an arrow record.
func contiguousParquetRowGroupToArrowRecord(
	ctx context.Context,
	pool memory.Allocator,
	rg parquet.RowGroup,
	projections []logicalplan.ColumnMatcher,
	filterExpr logicalplan.Expr,
	distinctColumns []logicalplan.ColumnMatcher,
) (arrow.Record, error) {
	s := rg.Schema()
	parquetColumns := rg.ColumnChunks()
	parquetFields := s.Fields()

	if len(distinctColumns) == 1 && filterExpr == nil {
		// We can use the faster path for a single distinct column by just
		// returning its dictionary.
		fields := make([]arrow.Field, 0, 1)
		cols := make([]arrow.Array, 0, 1)
		rows := int64(0)
		for i, field := range parquetFields {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				name := field.Name()
				if distinctColumns[0].Match(name) {
					typ, nullable, array, err := parquetColumnToArrowArray(
						pool,
						field,
						parquetColumns[i],
						true,
					)
					if err != nil {
						return nil, fmt.Errorf("convert parquet column to arrow array: %w", err)
					}
					fields = append(fields, arrow.Field{
						Name:     name,
						Type:     typ,
						Nullable: nullable,
					})
					cols = append(cols, array)
					rows = int64(array.Len())
				}
			}
		}

		schema := arrow.NewSchema(fields, nil)
		return array.NewRecord(schema, cols, rows), nil
	}

	fields := make([]arrow.Field, 0, len(parquetFields))
	cols := make([]arrow.Array, 0, len(parquetFields))

	for i, parquetField := range parquetFields {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if includedProjection(projections, parquetField.Name()) {
				typ, nullable, array, err := parquetColumnToArrowArray(
					pool,
					parquetField,
					parquetColumns[i],
					false,
				)
				if err != nil {
					return nil, fmt.Errorf("convert parquet column to arrow array: %w", err)
				}
				fields = append(fields, arrow.Field{
					Name:     parquetField.Name(),
					Type:     typ,
					Nullable: nullable,
				})
				cols = append(cols, array)
			}
		}
	}

	schema := arrow.NewSchema(fields, nil)
	return array.NewRecord(schema, cols, rg.NumRows()), nil
}

func includedProjection(projections []logicalplan.ColumnMatcher, name string) bool {
	if len(projections) == 0 {
		return true
	}

	for _, p := range projections {
		if p.Match(name) {
			return true
		}
	}
	return false
}

// parquetColumnToArrowArray converts a single parquet column to an arrow array
// and returns the type, nullability, and the actual resulting arrow array. If
// a column is a repeated type, it automatically boxes it into the appropriate
// arrow equivalent.
func parquetColumnToArrowArray(
	pool memory.Allocator,
	n parquet.Node,
	c parquet.ColumnChunk,
	dictionaryOnly bool,
) (
	arrow.DataType,
	bool,
	arrow.Array,
	error,
) {
	at, newValueWriter, err := parquetNodeToType(n)
	if err != nil {
		return nil, false, nil, err
	}

	var (
		w  valueWriter
		b  array.Builder
		lb *array.ListBuilder

		repeated = false
		nullable = false
	)

	optional := n.Optional()
	if optional {
		nullable = true
	}

	// Using the retrieved arrow type and whether the type is repeated we can
	// build a type-safe `writeValues` function that only casts the resulting
	// builder once and can perform optimized transfers of the page values to
	// the target array.
	if n.Repeated() {
		// If the column is repeated, we need to box it into a list.
		lt := arrow.ListOf(at)
		lt.SetElemNullable(optional)
		at = lt

		nullable = true
		repeated = true

		lb = array.NewBuilder(pool, at).(*array.ListBuilder)
		// A list builder actually expects all values of all sublists to be
		// written contiguously, the offsets where litsts start are recorded in
		// the offsets array below.
		w = newValueWriter(lb.ValueBuilder(), int(c.NumValues()))
		b = lb
	} else {
		b = array.NewBuilder(pool, at)
		w = newValueWriter(b, int(c.NumValues()))
	}
	defer b.Release()

	err = writePagesToArray(
		c.Pages(),
		optional,
		repeated,
		lb,
		w,
		dictionaryOnly,
	)
	if err != nil {
		return nil, false, nil, err
	}

	arr := b.NewArray()

	// Is this a bug in arrow? We already set the nullability above, but it
	// doesn't appear to transfer into the resulting array's type. Needs to be
	// investigated.
	switch t := arr.DataType().(type) {
	case *arrow.ListType:
		t.SetElemNullable(optional)
	}

	return at, nullable, arr, nil
}

// writePagesToArray reads all pages of a page iterator and writes the values
// to an array builder. If the type is a repeated type it will also write the
// starting offsets of lists to the list builder.
func writePagesToArray(
	pages parquet.Pages,
	optional bool,
	repeated bool,
	lb *array.ListBuilder,
	w valueWriter,
	dictionaryOnly bool,
) error {
	defer pages.Close()
	// We are potentially writing multiple pages to the same array, so we need
	// to keep track of the index of the offsets in case this is a List-type.
	i := 0
	for {
		p, err := pages.ReadPage()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read page: %w", err)
		}
		dict := p.Dictionary()

		switch {
		case !repeated && dictionaryOnly && dict != nil:
			// If we are only writing the dictionary, we don't need to read
			// the values.
			err = w.WritePage(dict.Page())
			if err != nil {
				return fmt.Errorf("write dictionary page: %w", err)
			}
		case !repeated && !optional && dict == nil:
			// If the column is not optional, we can read all values at once
			// consecutively without worrying about null values.
			err = w.WritePage(p)
			if err != nil {
				return fmt.Errorf("write page: %w", err)
			}
		default:
			values := make([]parquet.Value, p.NumValues())
			reader := p.Values()
			_, err = reader.ReadValues(values)
			// We're reading all values in the page so we always expect an io.EOF.
			if err != nil && err != io.EOF {
				return fmt.Errorf("read values: %w", err)
			}

			w.Write(values)

			if repeated {
				offsets := []int32{}
				validity := []bool{}
				for _, v := range values {
					rep := v.RepetitionLevel()
					def := v.DefinitionLevel()
					if rep == 0 && def == 1 {
						offsets = append(offsets, int32(i))
						validity = append(validity, true)
					}
					if rep == 0 && def == 0 {
						offsets = append(offsets, int32(i))
						validity = append(validity, false)
					}
					// rep == 1 && def == 1 means the item in the list is not null which is handled by the value writer
					// rep == 1 && def == 0 means the item in the list is null which is handled by the value writer
					i++
				}

				lb.AppendValues(offsets, validity)
			}
		}
	}

	return nil
}

// ParquetNodeToType converts a parquet node to an arrow type.
func ParquetNodeToType(n parquet.Node) (arrow.DataType, error) {
	typ, _, err := parquetNodeToType(n)
	if err != nil {
		return nil, err
	}
	return typ, nil
}

// parquetNodeToType converts a parquet node to an arrow type and a function to
// create a value writer.
func parquetNodeToType(n parquet.Node) (arrow.DataType, func(b array.Builder, numValues int) valueWriter, error) {
	t := n.Type()
	lt := t.LogicalType()

	if lt == nil {
		return nil, nil, errors.New("unsupported type")
	}

	switch {
	case lt.UTF8 != nil:
		return &arrow.BinaryType{}, newBinaryValueWriter, nil
	case lt.Integer != nil:
		switch lt.Integer.BitWidth {
		case 64:
			if lt.Integer.IsSigned {
				return &arrow.Int64Type{}, newInt64ValueWriter, nil
			}
			return &arrow.Uint64Type{}, newUint64ValueWriter, nil
		default:
			return nil, nil, errors.New("unsupported int bit width")
		}
	default:
		return nil, nil, errors.New("unsupported type")
	}
}

var ErrPageTypeMismatch = errors.New("page type mismatch")

type valueWriter interface {
	WritePage(p parquet.Page) error
	Write([]parquet.Value)
}

type binaryValueWriter struct {
	b          *array.BinaryBuilder
	numValues  int
	firstWrite bool
}

func newBinaryValueWriter(b array.Builder, numValues int) valueWriter {
	return &binaryValueWriter{
		b:          b.(*array.BinaryBuilder),
		numValues:  numValues,
		firstWrite: true,
	}
}

func (w *binaryValueWriter) Write(values []parquet.Value) {
	if w.firstWrite {
		w.firstWrite = false

		// Depending on the nullability of the column this could be optimized
		// further by reading strings directly and adding all of them at once
		// to the array builder.
		vs := make([][]byte, len(values))
		validity := make([]bool, len(values))
		largest := 0
		for i, v := range values {
			if !v.IsNull() {
				vs[i] = v.ByteArray()
				if len(vs[i]) > largest {
					largest = len(vs[i])
				}
				validity[i] = true
			}
		}
		w.b.ReserveData(w.numValues * largest)

		w.b.AppendValues(vs, validity)
	} else {
		// Depending on the nullability of the column this could be optimized
		// further by reading strings directly and adding all of them at once
		// to the array builder.
		vs := make([][]byte, len(values))
		validity := make([]bool, len(values))
		for i, v := range values {
			if !v.IsNull() {
				vs[i] = v.ByteArray()
				validity[i] = true
			}
		}

		w.b.AppendValues(vs, validity)
	}
}

// TODO: implement fast path of writing the whole page directly.
func (w *binaryValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}

type int64ValueWriter struct {
	b   *array.Int64Builder
	buf []int64
}

func newInt64ValueWriter(b array.Builder, numValues int) valueWriter {
	res := &int64ValueWriter{
		b: b.(*array.Int64Builder),
	}
	res.b.Reserve(numValues)
	return res
}

func (w *int64ValueWriter) Write(values []parquet.Value) {
	// Depending on the nullability of the column this could be optimized
	// further by reading int64s directly and adding all of them at once to
	// the array builder.
	for _, v := range values {
		if v.IsNull() {
			w.b.AppendNull()
		} else {
			w.b.Append(v.Int64())
		}
	}
}

func (w *int64ValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	ireader, ok := reader.(parquet.Int64Reader)
	if ok {
		// fast path
		if w.buf == nil {
			w.buf = make([]int64, p.NumValues())
		}
		values := w.buf
		for {
			n, err := ireader.ReadInt64s(values)
			if err != nil && err != io.EOF {
				return fmt.Errorf("read values: %w", err)
			}

			w.b.AppendValues(values[:n], nil)
			if err == io.EOF {
				break
			}
		}
		return nil
	}

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}

type uint64ValueWriter struct {
	b *array.Uint64Builder
}

func newUint64ValueWriter(b array.Builder, numValues int) valueWriter {
	res := &uint64ValueWriter{
		b: b.(*array.Uint64Builder),
	}
	res.b.Reserve(numValues)
	return res
}

func (w *uint64ValueWriter) Write(values []parquet.Value) {
	// Depending on the nullability of the column this could be optimized
	// further by reading uint64s directly and adding all of them at once
	// to the array builder.
	for _, v := range values {
		if v.IsNull() {
			w.b.AppendNull()
		} else {
			w.b.Append(uint64(v.Int64()))
		}
	}
}

// TODO: implement fast path of writing the whole page directly.
func (w *uint64ValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}

type repeatedValueWriter struct {
	b      *array.ListBuilder
	values valueWriter
}

func newListValueWriter(newValueWriter func(b array.Builder, numValues int) valueWriter) func(b array.Builder, numValues int) valueWriter {
	return func(b array.Builder, numValues int) valueWriter {
		builder := b.(*array.ListBuilder)

		return &repeatedValueWriter{
			b:      builder,
			values: newValueWriter(builder.ValueBuilder(), numValues),
		}
	}
}

func (w *repeatedValueWriter) Write(values []parquet.Value) {
	v0 := values[0]
	rep := v0.RepetitionLevel()
	def := v0.DefinitionLevel()
	if rep == 0 && def == 0 {
		w.b.AppendNull()
	}

	w.b.Append(true)
	w.values.Write(values)
}

// TODO: implement fast path of writing the whole page directly.
func (w *repeatedValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}
