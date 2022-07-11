package v1

import (
	"io"
	"sort"

	"github.com/segmentio/parquet-go"
)

type SortingColumns interface {
	parquet.RowGroupOption
	parquet.WriterOption
}

type Persister[T any] interface {
	Schema() *parquet.Schema
	Deconstruct(parquet.Row, uint64, T) parquet.Row
	Reconstruct(parquet.Row) (uint64, T, error)
	SortingColumns() SortingColumns
}

type Writer[T any, P Persister[T]] struct {
}

func (*Writer[T, P]) WriteParquetFile(file io.Writer, elements []T) error {
	var (
		persister P
		rows      = make([]parquet.Row, len(elements))
	)

	buffer := parquet.NewBuffer(persister.Schema(), persister.SortingColumns())

	for pos := range rows {
		rows[pos] = persister.Deconstruct(rows[pos], uint64(pos), elements[pos])
	}

	if _, err := buffer.WriteRows(rows); err != nil {
		return err
	}
	sort.Sort(buffer)

	writer := parquet.NewWriter(file, persister.Schema())
	if _, err := parquet.CopyRows(writer, buffer.Rows()); err != nil {
		return err
	}

	return writer.Close()
}
