package arrow

import (
	"github.com/apache/arrow/go/v12/arrow"
	"github.com/apache/arrow/go/v12/arrow/memory"
)

// ProfileArrowSchema defines the Arrow schema for profile data
// We split the hierarchical pprof structure into multiple record batches
type ProfileArrowSchema struct {
	// Metadata contains profile-level information
	TimeNanos         int64
	DurationNanos     int64
	Period            int64
	DropFrames        int64
	KeepFrames        int64
	DefaultSampleType int64

	// SampleTypes describes the value types in samples
	SampleTypes []ValueType
	PeriodType  ValueType
	Comments    []int64
}

type ValueType struct {
	Type int64 // Index into string table
	Unit int64 // Index into string table
}

// Arrow Schemas for each record batch
var (
	// SamplesSchema represents flattened sample data
	SamplesSchema = arrow.NewSchema([]arrow.Field{
		{Name: "sample_id", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "location_id", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "location_index", Type: arrow.PrimitiveTypes.Uint32, Nullable: false}, // Position in stack trace
		{Name: "value_index", Type: arrow.PrimitiveTypes.Uint32, Nullable: false},    // Which value type
		{Name: "value", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
		{Name: "label_key", Type: arrow.PrimitiveTypes.Int64, Nullable: true}, // Index into string table
		{Name: "label_str", Type: arrow.PrimitiveTypes.Int64, Nullable: true}, // Index into string table
		{Name: "label_num", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "label_num_unit", Type: arrow.PrimitiveTypes.Int64, Nullable: true}, // Index into string table
	}, nil)

	// LocationsSchema represents location data
	LocationsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "mapping_id", Type: arrow.PrimitiveTypes.Uint64, Nullable: true},
		{Name: "address", Type: arrow.PrimitiveTypes.Uint64, Nullable: true},
		{Name: "is_folded", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "function_id", Type: arrow.PrimitiveTypes.Uint64, Nullable: true},
		{Name: "line", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil)

	// FunctionsSchema represents function data
	FunctionsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "name", Type: arrow.PrimitiveTypes.Int64, Nullable: false},       // Index into string table
		{Name: "system_name", Type: arrow.PrimitiveTypes.Int64, Nullable: true}, // Index into string table
		{Name: "filename", Type: arrow.PrimitiveTypes.Int64, Nullable: true},    // Index into string table
		{Name: "start_line", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil)

	// MappingsSchema represents mapping data
	MappingsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "memory_start", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "memory_limit", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "file_offset", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "filename", Type: arrow.PrimitiveTypes.Int64, Nullable: true}, // Index into string table
		{Name: "build_id", Type: arrow.PrimitiveTypes.Int64, Nullable: true}, // Index into string table
		{Name: "has_functions", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "has_filenames", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "has_line_numbers", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "has_inline_frames", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
	}, nil)

	// StringsSchema represents the string table
	StringsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
		{Name: "value", Type: arrow.BinaryTypes.String, Nullable: false},
	}, nil)
)

// ProfileArrowData would hold the Arrow record batches for a profile, but
// we're using the protobuf format for transport instead. This type is
// kept for potential future use in direct Arrow transfer methods.

// NewMemoryPool creates a new Arrow memory pool
func NewMemoryPool() memory.Allocator {
	return memory.NewGoAllocator()
}
