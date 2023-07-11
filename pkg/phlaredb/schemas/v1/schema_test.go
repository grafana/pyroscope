package v1

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
)

// This test ensures that the structs that are stored and the used schema matches
func TestSchemaMatch(t *testing.T) {
	// TODO: Unfortunately the upstream schema doesn't correctly produce a
	// schema of a List of a struct pointer. This replaces this in the schema
	// comparison, because this has no affect to our construct/reconstruct code
	// we can simply replace the string in the schema.
	profilesStructSchema := strings.ReplaceAll(
		parquet.SchemaOf(&Profile{}).String(),
		"optional group element",
		"required group element",
	)

	require.Equal(t, profilesStructSchema, profilesSchema.String())

	stacktracesStructSchema := parquet.SchemaOf(&storedStacktrace{})
	require.Equal(t, strings.Replace(stacktracesStructSchema.String(), "message storedStacktrace", "message Stacktrace", 1), stacktracesSchema.String())

	stringsStructSchema := parquet.SchemaOf(&StoredString{})
	require.Equal(t, strings.Replace(stringsStructSchema.String(), "message StoredString", "message String", 1), stringsSchema.String())
}

func newStacktraces() []*Stacktrace {
	return []*Stacktrace{
		{LocationIDs: []uint64{0x11}},
		{LocationIDs: []uint64{}},
		{LocationIDs: []uint64{12, 13}},
		{LocationIDs: []uint64{}},
		{LocationIDs: []uint64{14, 15}},
	}
}

func TestStacktracesRoundTrip(t *testing.T) {
	var (
		s   = newStacktraces()
		w   = &ReadWriter[*Stacktrace, *StacktracePersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, s))

	sRead, err := w.ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, newStacktraces(), sRead)
}

func newStrings() []string {
	return []string{
		"",
		"foo",
		"bar",
		"baz",
		"",
	}
}

func TestStringsRoundTrip(t *testing.T) {
	var (
		s   = newStrings()
		w   = &ReadWriter[string, *StringPersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, s))

	sRead, err := w.ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, newStrings(), sRead)
}

func newProfiles() []*Profile {
	return []*Profile{
		{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			TimeNanos:   1001,
			SeriesIndex: 0xaa,
			Samples: []*Sample{
				{
					StacktraceID: 0xba,
					Value:        0xca,
					Labels:       []*profilev1.Label{},
				},
				{
					StacktraceID: 0xbb,
					Value:        0xca,
					Labels: []*profilev1.Label{
						{Key: 0xda, Str: 0xea},
					},
				},
			},
			Comments: []int64{},
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			TimeNanos:   1001,
			SeriesIndex: 0xab,
			Samples: []*Sample{
				{
					StacktraceID: 0xba,
					Value:        0xcc,
					Labels:       []*profilev1.Label{},
				},
				{
					StacktraceID: 0xbb,
					Value:        0xcc,
					Labels: []*profilev1.Label{
						{Key: 0xda, Str: 0xea},
					},
				},
			},
			Comments: []int64{},
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			SeriesIndex: 0xab,
			TimeNanos:   1002,
			Samples: []*Sample{
				{
					StacktraceID: 0xbc,
					Value:        0xcd,
					Labels:       []*profilev1.Label{},
				},
			},
			Comments: []int64{},
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			SeriesIndex: 0xac,
			TimeNanos:   1002,
			Samples: []*Sample{
				{
					StacktraceID: 0xbc,
					Value:        0xce,
					Labels:       []*profilev1.Label{},
				},
			},
			Comments: []int64{},
		},
	}
}

func TestProfilesRoundTrip(t *testing.T) {
	var (
		p   = newProfiles()
		w   = &ReadWriter[*Profile, *ProfilePersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, p))

	sRead, err := w.ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, newProfiles(), sRead)
}

func TestLocationsRoundTrip(t *testing.T) {
	raw := []*profilev1.Location{
		{
			Id:        8,
			Address:   9,
			MappingId: 10,
			Line: []*profilev1.Line{
				{
					FunctionId: 11,
					Line:       12,
				},
				{
					FunctionId: 13,
					Line:       14,
				},
			},
			IsFolded: true,
		},
		{
			Id:        1,
			Address:   2,
			MappingId: 3,
			Line: []*profilev1.Line{
				{
					FunctionId: 4,
					Line:       5,
				},
				{
					FunctionId: 6,
					Line:       7,
				},
			},
			IsFolded: false,
		},
	}

	mem := []*InMemoryLocation{
		{
			Id:        8,
			Address:   9,
			MappingId: 10,
			Line: []InMemoryLine{
				{
					FunctionId: 11,
					Line:       12,
				},
				{
					FunctionId: 13,
					Line:       14,
				},
			},
			IsFolded: true,
		},
		{
			Id:        1,
			Address:   2,
			MappingId: 3,
			Line: []InMemoryLine{
				{
					FunctionId: 4,
					Line:       5,
				},
				{
					FunctionId: 6,
					Line:       7,
				},
			},
			IsFolded: false,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, new(ReadWriter[*profilev1.Location, *pprofLocationPersister]).WriteParquetFile(&buf, raw))
	actual, err := new(ReadWriter[*InMemoryLocation, *LocationPersister]).ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, mem, actual)

	buf.Reset()
	require.NoError(t, new(ReadWriter[*InMemoryLocation, *LocationPersister]).WriteParquetFile(&buf, mem))
	actual, err = new(ReadWriter[*InMemoryLocation, *LocationPersister]).ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, mem, actual)
}

var protoLocationsSchema = parquet.SchemaOf(&profilev1.Location{})

type pprofLocationPersister struct{}

func (*pprofLocationPersister) Name() string { return "locations" }

func (*pprofLocationPersister) Schema() *parquet.Schema { return protoLocationsSchema }

func (*pprofLocationPersister) SortingColumns() parquet.SortingOption {
	return parquet.SortingColumns()
}

func (*pprofLocationPersister) Deconstruct(row parquet.Row, _ uint64, loc *profilev1.Location) parquet.Row {
	row = protoLocationsSchema.Deconstruct(row, loc)
	return row
}

func (*pprofLocationPersister) Reconstruct(row parquet.Row) (uint64, *profilev1.Location, error) {
	var loc profilev1.Location
	if err := protoLocationsSchema.Reconstruct(&loc, row); err != nil {
		return 0, nil, err
	}
	return 0, &loc, nil
}

func TestFunctionsRoundTrip(t *testing.T) {
	raw := []*profilev1.Function{
		{
			Id:         6,
			Name:       7,
			SystemName: 8,
			Filename:   9,
			StartLine:  10,
		},
		{
			Id:         1,
			Name:       2,
			SystemName: 3,
			Filename:   4,
			StartLine:  5,
		},
	}

	mem := []*InMemoryFunction{
		{
			Id:         6,
			Name:       7,
			SystemName: 8,
			Filename:   9,
			StartLine:  10,
		},
		{
			Id:         1,
			Name:       2,
			SystemName: 3,
			Filename:   4,
			StartLine:  5,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, new(ReadWriter[*profilev1.Function, *pprofFunctionPersister]).WriteParquetFile(&buf, raw))
	actual, err := new(ReadWriter[*InMemoryFunction, *FunctionPersister]).ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, mem, actual)

	buf.Reset()
	require.NoError(t, new(ReadWriter[*InMemoryFunction, *FunctionPersister]).WriteParquetFile(&buf, mem))
	actual, err = new(ReadWriter[*InMemoryFunction, *FunctionPersister]).ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, mem, actual)
}

var protoFunctionSchema = parquet.SchemaOf(&profilev1.Function{})

type pprofFunctionPersister struct{}

func (*pprofFunctionPersister) Name() string { return "functions" }

func (*pprofFunctionPersister) Schema() *parquet.Schema { return protoFunctionSchema }

func (*pprofFunctionPersister) SortingColumns() parquet.SortingOption {
	return parquet.SortingColumns()
}

func (*pprofFunctionPersister) Deconstruct(row parquet.Row, _ uint64, loc *profilev1.Function) parquet.Row {
	row = protoFunctionSchema.Deconstruct(row, loc)
	return row
}

func (*pprofFunctionPersister) Reconstruct(row parquet.Row) (uint64, *profilev1.Function, error) {
	var fn profilev1.Function
	if err := protoFunctionSchema.Reconstruct(&fn, row); err != nil {
		return 0, nil, err
	}
	return 0, &fn, nil
}

func TestMappingsRoundTrip(t *testing.T) {
	raw := []*profilev1.Mapping{
		{
			Id:              7,
			MemoryStart:     8,
			MemoryLimit:     9,
			FileOffset:      10,
			Filename:        11,
			BuildId:         12,
			HasFunctions:    true,
			HasFilenames:    false,
			HasLineNumbers:  true,
			HasInlineFrames: false,
		},
		{
			Id:              1,
			MemoryStart:     2,
			MemoryLimit:     3,
			FileOffset:      4,
			Filename:        5,
			BuildId:         6,
			HasFunctions:    false,
			HasFilenames:    true,
			HasLineNumbers:  false,
			HasInlineFrames: true,
		},
	}

	mem := []*InMemoryMapping{
		{
			Id:              7,
			MemoryStart:     8,
			MemoryLimit:     9,
			FileOffset:      10,
			Filename:        11,
			BuildId:         12,
			HasFunctions:    true,
			HasFilenames:    false,
			HasLineNumbers:  true,
			HasInlineFrames: false,
		},
		{
			Id:              1,
			MemoryStart:     2,
			MemoryLimit:     3,
			FileOffset:      4,
			Filename:        5,
			BuildId:         6,
			HasFunctions:    false,
			HasFilenames:    true,
			HasLineNumbers:  false,
			HasInlineFrames: true,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, new(ReadWriter[*profilev1.Mapping, *pprofMappingPersister]).WriteParquetFile(&buf, raw))
	actual, err := new(ReadWriter[*InMemoryMapping, *MappingPersister]).ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, mem, actual)

	//	buf.Reset()
	//	require.NoError(t, new(ReadWriter[*InMemoryMapping, *MappingPersister]).WriteParquetFile(&buf, mem))
	//	actual, err = new(ReadWriter[*InMemoryMapping, *MappingPersister]).ReadParquetFile(bytes.NewReader(buf.Bytes()))
	//	require.NoError(t, err)
	//	assert.Equal(t, mem, actual)
}

var protoMappingSchema = parquet.SchemaOf(&profilev1.Mapping{})

type pprofMappingPersister struct{}

func (*pprofMappingPersister) Name() string { return "mappings" }

func (*pprofMappingPersister) Schema() *parquet.Schema { return protoMappingSchema }

func (*pprofMappingPersister) SortingColumns() parquet.SortingOption { return parquet.SortingColumns() }

func (*pprofMappingPersister) Deconstruct(row parquet.Row, _ uint64, loc *profilev1.Mapping) parquet.Row {
	row = protoMappingSchema.Deconstruct(row, loc)
	return row
}

func (*pprofMappingPersister) Reconstruct(row parquet.Row) (uint64, *profilev1.Mapping, error) {
	var m profilev1.Mapping
	if err := protoMappingSchema.Reconstruct(&m, row); err != nil {
		return 0, nil, err
	}
	return 0, &m, nil
}
