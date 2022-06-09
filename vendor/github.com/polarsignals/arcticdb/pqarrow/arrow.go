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
	"github.com/polarsignals/arcticdb/pqarrow/convert"
	"github.com/polarsignals/arcticdb/pqarrow/writer"
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
	newWriterFuncs := make([]func(array.Builder, int) writer.ValueWriter, 0, len(parquetFields))
	for _, parquetField := range parquetFields {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			name := parquetField.Name()
			node := dynparquet.FieldByName(s, name)
			typ, newValueWriter, err := convert.ParquetNodeToTypeWithWriterFunc(node)
			if err != nil {
				return nil, err
			}
			nullable := false
			if node.Optional() {
				nullable = true
			}

			if node.Repeated() {
				typ = arrow.ListOf(typ)
				newValueWriter = writer.NewListValueWriter(newValueWriter)
			}
			newWriterFuncs = append(newWriterFuncs, newValueWriter)

			fields = append(fields, arrow.Field{
				Name:     name,
				Type:     typ,
				Nullable: nullable,
			})
		}
	}

	writers := make([]writer.ValueWriter, len(parquetFields))
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
	at, newValueWriter, err := convert.ParquetNodeToTypeWithWriterFunc(n)
	if err != nil {
		return nil, false, nil, err
	}

	var (
		w  writer.ValueWriter
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
	w writer.ValueWriter,
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

var ErrPageTypeMismatch = errors.New("page type mismatch")
