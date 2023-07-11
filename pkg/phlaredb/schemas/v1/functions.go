package v1

import (
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
)

var functionsSchema = parquet.SchemaOf(new(profilev1.Function))

type FunctionPersister struct{}

func (*FunctionPersister) Name() string { return "functions" }

func (*FunctionPersister) Schema() *parquet.Schema { return functionsSchema }

func (*FunctionPersister) SortingColumns() parquet.SortingOption { return parquet.SortingColumns() }

func (*FunctionPersister) Deconstruct(row parquet.Row, _ uint64, fn *InMemoryFunction) parquet.Row {
	if cap(row) < 5 {
		row = make(parquet.Row, 0, 5)
	}
	row = row[:0]
	row = append(row, parquet.Int64Value(int64(fn.Id)).Level(0, 0, 0))
	row = append(row, parquet.Int32Value(int32(fn.Name)).Level(0, 0, 1))
	row = append(row, parquet.Int32Value(int32(fn.SystemName)).Level(0, 0, 2))
	row = append(row, parquet.Int32Value(int32(fn.Filename)).Level(0, 0, 3))
	row = append(row, parquet.Int32Value(int32(fn.StartLine)).Level(0, 0, 4))
	return row
}

func (*FunctionPersister) Reconstruct(row parquet.Row) (uint64, *InMemoryFunction, error) {
	loc := InMemoryFunction{
		Id:         row[0].Uint64(),
		Name:       row[1].Uint32(),
		SystemName: row[2].Uint32(),
		Filename:   row[3].Uint32(),
		StartLine:  row[4].Uint32(),
	}
	return 0, &loc, nil
}

type InMemoryFunction struct {
	// Unique nonzero id for the function.
	Id uint64
	// Name of the function, in human-readable form if available.
	Name uint32
	// Name of the function, as identified by the system.
	// For instance, it can be a C++ mangled name.
	SystemName uint32
	// Source file containing the function.
	Filename uint32
	// Line number in source file.
	StartLine uint32
}
