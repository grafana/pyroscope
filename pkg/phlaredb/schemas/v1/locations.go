package v1

import (
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
)

var locationsSchema = parquet.SchemaOf(new(profilev1.Location))

type LocationPersister struct{}

func (*LocationPersister) Name() string { return "locations" }

func (*LocationPersister) Schema() *parquet.Schema { return locationsSchema }

func (*LocationPersister) SortingColumns() parquet.SortingOption { return parquet.SortingColumns() }

func (*LocationPersister) Deconstruct(row parquet.Row, _ uint64, loc *InMemoryLocation) parquet.Row {
	var (
		col    = -1
		newCol = func() int {
			col++
			return col
		}
		totalCols = 4 + (2 * len(loc.Line))
	)
	if cap(row) < totalCols {
		row = make(parquet.Row, 0, totalCols)
	}
	row = row[:0]
	row = append(row, parquet.Int64Value(int64(loc.Id)).Level(0, 0, newCol()))
	row = append(row, parquet.Int32Value(int32(loc.MappingId)).Level(0, 0, newCol()))
	row = append(row, parquet.Int64Value(int64(loc.Address)).Level(0, 0, newCol()))

	newCol()
	if len(loc.Line) == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, col))
	}
	repetition := -1
	for i := range loc.Line {
		if repetition < 1 {
			repetition++
		}
		row = append(row, parquet.Int32Value(int32(loc.Line[i].FunctionId)).Level(repetition, 1, col))
	}

	newCol()
	if len(loc.Line) == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, col))
	}
	repetition = -1
	for i := range loc.Line {
		if repetition < 1 {
			repetition++
		}
		row = append(row, parquet.Int32Value(loc.Line[i].Line).Level(repetition, 1, col))
	}

	row = append(row, parquet.BooleanValue(loc.IsFolded).Level(0, 0, newCol()))
	return row
}

func (*LocationPersister) Reconstruct(row parquet.Row) (uint64, *InMemoryLocation, error) {
	loc := InMemoryLocation{
		Id:        row[0].Uint64(),
		MappingId: uint32(row[1].Uint64()),
		Address:   row[2].Uint64(),
		IsFolded:  row[len(row)-1].Boolean(),
	}
	lines := row[3 : len(row)-1]
	loc.Line = make([]InMemoryLine, len(lines)/2)
	for i, v := range lines[:len(lines)/2] {
		loc.Line[i].FunctionId = uint32(v.Uint64())
	}
	for i, v := range lines[len(lines)/2:] {
		loc.Line[i].Line = int32(v.Uint64())
	}
	return 0, &loc, nil
}

type InMemoryLocation struct {
	// Unique nonzero id for the location.  A profile could use
	// instruction addresses or any integer sequence as ids.
	Id uint64
	// The instruction address for this location, if available.  It
	// should be within [Mapping.memory_start...Mapping.memory_limit]
	// for the corresponding mapping. A non-leaf address may be in the
	// middle of a call instruction. It is up to display tools to find
	// the beginning of the instruction if necessary.
	Address uint64
	// The id of the corresponding profile.Mapping for this location.
	// It can be unset if the mapping is unknown or not applicable for
	// this profile type.
	MappingId uint32
	// Provides an indication that multiple symbols map to this location's
	// address, for example due to identical code folding by the linker. In that
	// case the line information above represents one of the multiple
	// symbols. This field must be recomputed when the symbolization state of the
	// profile changes.
	IsFolded bool
	// Multiple line indicates this location has inlined functions,
	// where the last entry represents the caller into which the
	// preceding entries were inlined.
	//
	// E.g., if memcpy() is inlined into printf:
	//
	//	line[0].function_name == "memcpy"
	//	line[1].function_name == "printf"
	Line []InMemoryLine
}

type InMemoryLine struct {
	// The id of the corresponding profile.Function for this line.
	FunctionId uint32
	// Line number in source code.
	Line int32
}
