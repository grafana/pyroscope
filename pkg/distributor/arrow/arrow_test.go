package arrow

import (
	"testing"

	"github.com/apache/arrow/go/v12/arrow/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// TestArrowRoundTrip verifies that profile data is preserved exactly
// through Arrow serialization and deserialization
func TestArrowRoundTrip(t *testing.T) {
	// Create a test profile with specific structure that mimics the failing test
	original := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2}, // cpu nanoseconds
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2, 3, 4}, // Specific order that was failing
				Value:      []int64{100},
				Label: []*profilev1.Label{
					{Key: 1, Str: 3},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:              1,
				MemoryStart:     0x1000,
				MemoryLimit:     0x2000,
				FileOffset:      0,
				Filename:        4,
				BuildId:         5,
				HasFunctions:    true,
				HasFilenames:    true,
				HasLineNumbers:  true,
				HasInlineFrames: false,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   0x1500,
				Line: []*profilev1.Line{
					{FunctionId: 3, Line: 0},
				},
				IsFolded: false,
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   0x1600,
				Line: []*profilev1.Line{
					{FunctionId: 4, Line: 0},
				},
				IsFolded: false,
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   0x1700,
				Line: []*profilev1.Line{
					{FunctionId: 3, Line: 0},
				},
				IsFolded: false,
			},
			{
				Id:        4,
				MappingId: 1,
				Address:   0x1800,
				Line: []*profilev1.Line{
					{FunctionId: 4, Line: 0},
				},
				IsFolded: false,
			},
		},
		Function: []*profilev1.Function{
			{Id: 3, Name: 6, SystemName: 0, Filename: 7, StartLine: 0},
			{Id: 4, Name: 8, SystemName: 0, Filename: 7, StartLine: 0},
		},
		StringTable: []string{
			"",
			"cpu",
			"nanoseconds",
			"service_name",
			"/usr/lib/libfoo.so",
			"2fa2055ef20fabc972d5751147e093275514b142",
			"main",
			"/path/to/file.c",
			"atoll_b",
		},
		TimeNanos:     0,
		DurationNanos: 0,
		PeriodType:    &profilev1.ValueType{Type: 1, Unit: 2},
		Period:        1000000000,
		Comment:       []int64{},
	}

	pool := memory.NewGoAllocator()

	// Convert to Arrow format
	arrowData, err := ProfileToArrow(original, pool)
	require.NoError(t, err, "Failed to convert profile to Arrow format")
	require.NotNil(t, arrowData, "Arrow data should not be nil")

	// Convert back to profile format
	reconstructed, err := ArrowToProfile(arrowData, pool)
	require.NoError(t, err, "Failed to convert Arrow back to profile")
	require.NotNil(t, reconstructed, "Reconstructed profile should not be nil")

	// Verify exact equality of critical structures

	// Check samples
	assert.Equal(t, len(original.Sample), len(reconstructed.Sample), "Sample count should match")
	if len(original.Sample) > 0 && len(reconstructed.Sample) > 0 {
		origSample := original.Sample[0]
		recoSample := reconstructed.Sample[0]

		// This was the failing assertion - location IDs must be in exact same order
		assert.Equal(t, origSample.LocationId, recoSample.LocationId, "Location IDs must be in exact same order")
		assert.Equal(t, origSample.Value, recoSample.Value, "Values should match")
		assert.Equal(t, len(origSample.Label), len(recoSample.Label), "Label count should match")
	}

	// Check locations - IDs and order
	assert.Equal(t, len(original.Location), len(reconstructed.Location), "Location count should match")
	for i := range original.Location {
		assert.Equal(t, original.Location[i].Id, reconstructed.Location[i].Id,
			"Location ID should match at index %d", i)
		assert.Equal(t, original.Location[i].MappingId, reconstructed.Location[i].MappingId,
			"Location MappingId should match at index %d", i)
		assert.Equal(t, original.Location[i].Address, reconstructed.Location[i].Address,
			"Location Address should match at index %d", i)
	}

	// Check functions - IDs and order
	assert.Equal(t, len(original.Function), len(reconstructed.Function), "Function count should match")
	for i := range original.Function {
		assert.Equal(t, original.Function[i].Id, reconstructed.Function[i].Id,
			"Function ID should match at index %d", i)
		assert.Equal(t, original.Function[i].Name, reconstructed.Function[i].Name,
			"Function Name should match at index %d", i)
	}

	// Check mappings - IDs and order
	assert.Equal(t, len(original.Mapping), len(reconstructed.Mapping), "Mapping count should match")
	for i := range original.Mapping {
		assert.Equal(t, original.Mapping[i].Id, reconstructed.Mapping[i].Id,
			"Mapping ID should match at index %d", i)
	}

	// Check string table
	assert.Equal(t, original.StringTable, reconstructed.StringTable, "String table should match exactly")

	// Check other metadata
	assert.Equal(t, original.TimeNanos, reconstructed.TimeNanos, "TimeNanos should match")
	assert.Equal(t, original.DurationNanos, reconstructed.DurationNanos, "DurationNanos should match")
	assert.Equal(t, original.Period, reconstructed.Period, "Period should match")
}

