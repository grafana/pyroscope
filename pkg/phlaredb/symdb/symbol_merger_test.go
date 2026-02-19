package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

const expectedTreeSymbols = `
{
  "mappings": [
    {},
    {
      "id": "1",
      "filename": "3",
      "memoryLimit": "8192",
      "memoryStart": "4096"
    }
  ],
  "locations": [
    {},
    {
      "id": "1",
      "address": "4660",
      "mappingId": "1",
      "line": [
        {
          "functionId": "1",
          "line": "42"
        }
      ]
    },
    {
      "address": "22136",
      "mappingId": "1",
      "id": "2"
    }
  ],
  "functions": [
    {},
    {
      "filename": "2",
      "id": "1",
      "name": "1",
      "startLine": "10",
      "systemName": "1"
    }
  ],
  "strings": [
    "",
    "main",
    "/path/to/file.go",
    "executable"
  ],
  "mappingHashes": [
    "17241709254077376921",
    "17123522043250476816"
  ],
  "locationHashes": [
    "17241709254077376921",
    "2168997802985953269",
    "18320325443818765556"
  ],
  "functionHashes": [
    "17241709254077376921",
    "3093208752845406239"
  ],
  "stringHashes": [
    "17241709254077376921",
    "1164858042786835974",
    "16187834116879249968",
    "1384254427617264253"
  ]
}
`

func createTestSymbols() *Symbols {
	return &Symbols{
		Locations: []schemav1.InMemoryLocation{
			{
				MappingId: 0,
				Address:   0x1234,
				Line: []schemav1.InMemoryLine{
					{FunctionId: 0, Line: 42},
				},
			},
			{
				Address: 0x5678,
			},
		},
		Mappings: []schemav1.InMemoryMapping{
			{
				MemoryStart: 0x1000,
				MemoryLimit: 0x2000,
				FileOffset:  0,
				Filename:    3,
				BuildId:     0,
			},
		},
		Functions: []schemav1.InMemoryFunction{
			{Name: 1, SystemName: 1, Filename: 2, StartLine: 10},
		},
		Strings: []string{"", "main", "/path/to/file.go", "executable"},
	}
}

func TestSymbolMerger(t *testing.T) {
	// Create a merger and add test symbols
	merger := NewSymbolMerger()
	ts := createTestSymbols()

	locIDs := []int32{0, 1}

	keepAll := func(f func(model.LocationRefName) model.LocationRefName) {
		for _, locID := range locIDs {
			require.Equal(t, model.LocationRefName(locID+1), f(model.LocationRefName(locID+1)))
		}
	}

	adder, err := merger.addSymbols(ts, locIDs)
	require.NoError(t, err)
	// adder renumbers from 0 to 1
	for _, locID := range locIDs {
		require.Equal(t, model.LocationRefName(locID+1), adder(model.LocationRefName(locID)))
	}

	// Build the result, keeping all locations
	builder := merger.ResultBuilder()
	keepAll(builder.KeepSymbol)

	result := &queryv1.TreeSymbols{}
	builder.Build(result)

	// compare TreeSymbols
	b, err := protojson.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, expectedTreeSymbols, string(b))

	// now merge them twice
	merger = NewSymbolMerger()
	_, err = merger.Add(result)
	require.NoError(t, err)
	adder, err = merger.Add(result)
	require.NoError(t, err)
	keepAll(adder)
	adder, err = merger.Add(result)
	require.NoError(t, err)
	keepAll(adder)

	// Build the result, keeping all locations
	builder = merger.ResultBuilder()
	keepAll(builder.KeepSymbol)

	result = &queryv1.TreeSymbols{}
	builder.Build(result)

	// compare TreeSymbols
	b, err = protojson.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, expectedTreeSymbols, string(b))
}

func TestSymbolMerger_HashCollision(t *testing.T) {
	sm := NewSymbolMerger()

	const sameHash = uint64(0xdeadbeefcafebabe)

	// Add "foo" – placed at index 1 (index 0 is the sentinel empty string).
	idxFoo := sm.strings.add(sameHash, "foo")
	// Add "bar" with the same hash – linear probing must resolve the collision.
	idxBar := sm.strings.add(sameHash, "bar")

	// Both values must get distinct indices.
	require.NotEqual(t, idxFoo, idxBar)
	require.Equal(t, int32(1), idxFoo)
	require.Equal(t, int32(2), idxBar)

	// Slice contains sentinel + the two colliding values.
	require.Equal(t, []string{"", "foo", "bar"}, sm.strings.sl)

	// The original hash (not the probe offset) is stored for both entries.
	require.Equal(t, sameHash, sm.strings.hashes[idxFoo])
	require.Equal(t, sameHash, sm.strings.hashes[idxBar])

	// Re-adding "foo" or "bar" with the same hash must return the original index (deduplication).
	idxFooAgain := sm.strings.add(sameHash, "foo")
	require.Equal(t, idxFoo, idxFooAgain)
	idxBarAgain := sm.strings.add(sameHash, "bar")
	require.Equal(t, idxBar, idxBarAgain)
}

func BenchmarkSymbolMerger(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/big-profile.pb.gz"}})

	b.Run("addSymbols", func(b *testing.B) {
		// Get symbols from the profile
		symbols := extractSymbolsFromProfile(b, s.profiles[0])

		// Collect all location IDs
		locationIDs := make([]int32, len(symbols.Locations))
		for i := range symbols.Locations {
			locationIDs[i] = int32(i)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			merger := NewSymbolMerger()
			_, err := merger.addSymbols(symbols, locationIDs)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("addSymbols_and_build", func(b *testing.B) {
		// Get symbols from the profile
		symbols := extractSymbolsFromProfile(b, s.profiles[0])

		// Collect all location IDs
		locationIDs := make([]int32, len(symbols.Locations))
		for i := range symbols.Locations {
			locationIDs[i] = int32(i)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			merger := NewSymbolMerger()
			remap, err := merger.addSymbols(symbols, locationIDs)
			if err != nil {
				b.Fatal(err)
			}

			builder := merger.ResultBuilder()
			for _, locID := range locationIDs {
				builder.KeepSymbol(remap(model.LocationRefName(locID)))
			}

			result := &queryv1.TreeSymbols{}
			builder.Build(result)
		}
	})

	b.Run("merge_multiple", func(b *testing.B) {
		// Get symbols from the profile
		symbols := extractSymbolsFromProfile(b, s.profiles[0])

		// Collect all location IDs
		locationIDs := make([]int32, len(symbols.Locations))
		for i := range symbols.Locations {
			locationIDs[i] = int32(i)
		}

		// Pre-create TreeSymbols for merging
		merger := NewSymbolMerger()
		remap, err := merger.addSymbols(symbols, locationIDs)
		require.NoError(b, err)

		builder := merger.ResultBuilder()
		for _, locID := range locationIDs {
			builder.KeepSymbol(remap(model.LocationRefName(locID)))
		}

		treeSymbols := &queryv1.TreeSymbols{}
		builder.Build(treeSymbols)

		b.Run("2_sources", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				merger := NewSymbolMerger()
				_, err := merger.Add(treeSymbols)
				if err != nil {
					b.Fatal(err)
				}
				_, err = merger.Add(treeSymbols)
				if err != nil {
					b.Fatal(err)
				}

				builder := merger.ResultBuilder()
				for j := int32(0); j < int32(len(treeSymbols.Locations)); j++ {
					builder.KeepSymbol(model.LocationRefName(j))
				}

				result := &queryv1.TreeSymbols{}
				builder.Build(result)
			}
		})

		b.Run("4_sources", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				merger := NewSymbolMerger()
				for j := 0; j < 4; j++ {
					_, err := merger.Add(treeSymbols)
					if err != nil {
						b.Fatal(err)
					}
				}

				builder := merger.ResultBuilder()
				for j := int32(0); j < int32(len(treeSymbols.Locations)); j++ {
					builder.KeepSymbol(model.LocationRefName(j))
				}

				result := &queryv1.TreeSymbols{}
				builder.Build(result)
			}
		})

		b.Run("8_sources", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				merger := NewSymbolMerger()
				for j := 0; j < 8; j++ {
					_, err := merger.Add(treeSymbols)
					if err != nil {
						b.Fatal(err)
					}
				}

				builder := merger.ResultBuilder()
				for j := int32(0); j < int32(len(treeSymbols.Locations)); j++ {
					builder.KeepSymbol(model.LocationRefName(j))
				}

				result := &queryv1.TreeSymbols{}
				builder.Build(result)
			}
		})
	})
}

func extractSymbolsFromProfile(tb testing.TB, profile *googlev1.Profile) *Symbols {
	tb.Helper()

	// Build location mapping
	locationMap := make(map[uint64]int32)
	for i, loc := range profile.Location {
		locationMap[loc.Id] = int32(i)
	}

	// Build mapping
	mappingMap := make(map[uint64]int32)
	for i, m := range profile.Mapping {
		mappingMap[m.Id] = int32(i)
	}

	// Build function mapping
	functionMap := make(map[uint64]int32)
	for i, f := range profile.Function {
		functionMap[f.Id] = int32(i)
	}

	symbols := &Symbols{
		Strings:   profile.StringTable,
		Locations: make([]schemav1.InMemoryLocation, len(profile.Location)),
		Mappings:  make([]schemav1.InMemoryMapping, len(profile.Mapping)),
		Functions: make([]schemav1.InMemoryFunction, len(profile.Function)),
	}

	// Convert mappings
	for i, m := range profile.Mapping {
		symbols.Mappings[i] = schemav1.InMemoryMapping{
			MemoryStart:     m.MemoryStart,
			MemoryLimit:     m.MemoryLimit,
			FileOffset:      m.FileOffset,
			Filename:        uint32(m.Filename),
			BuildId:         uint32(m.BuildId),
			HasFunctions:    m.HasFunctions,
			HasFilenames:    m.HasFilenames,
			HasLineNumbers:  m.HasLineNumbers,
			HasInlineFrames: m.HasInlineFrames,
		}
	}

	// Convert functions
	for i, f := range profile.Function {
		symbols.Functions[i] = schemav1.InMemoryFunction{
			Name:       uint32(f.Name),
			SystemName: uint32(f.SystemName),
			Filename:   uint32(f.Filename),
			StartLine:  uint32(f.StartLine),
		}
	}

	// Convert locations
	for i, loc := range profile.Location {
		mappingID := uint32(0)
		if loc.MappingId != 0 {
			mappingID = uint32(mappingMap[loc.MappingId])
		}

		lines := make([]schemav1.InMemoryLine, len(loc.Line))
		for j, line := range loc.Line {
			functionID := uint32(0)
			if line.FunctionId != 0 {
				functionID = uint32(functionMap[line.FunctionId])
			}
			lines[j] = schemav1.InMemoryLine{
				FunctionId: functionID,
				Line:       int32(line.Line),
			}
		}

		symbols.Locations[i] = schemav1.InMemoryLocation{
			MappingId: mappingID,
			Address:   loc.Address,
			IsFolded:  loc.IsFolded,
			Line:      lines,
		}
	}

	return symbols
}
