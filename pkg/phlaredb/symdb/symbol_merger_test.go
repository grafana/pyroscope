package symdb

import (
	"fmt"
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
    {
      "filename": "1",
      "memoryLimit": "8192",
      "memoryStart": "4096"
    }
  ],
  "locations": [
    {
      "address": "4660",
      "line": [
        {
          "line": "42"
        }
      ]
    },
    {
      "id": "1",
      "address": "22136"
    }
  ],
  "functions": [
    {
      "filename": "3",
      "name": "2",
      "startLine": "10",
      "systemName": "2"
    }
  ],
  "strings": [
    "",
    "executable",
    "main",
    "/path/to/file.go"
  ]
}
`

func TestSymbolMerger(t *testing.T) {
	// Create a merger and add test symbols
	merger := NewSymbolMerger()
	ts := createTestSymbols()

	keepAll := func(f func(model.LocationRefName) model.LocationRefName) {
		require.Equal(t, model.LocationRefName(0), f(model.LocationRefName(0)))
		require.Equal(t, model.LocationRefName(1), f(model.LocationRefName(1)))
	}

	keepAll(merger.addSymbols(ts))

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
	merger.Add(result)
	keepAll(merger.Add(result))
	keepAll(merger.Add(result))

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

// TestSymbolMerger_TwoDistinctProfiles validates that merging two different symbol tables
// correctly preserves the symbols and remaps locations appropriately
func TestSymbolMerger_TwoDistinctProfiles(t *testing.T) {
	// Create two different symbol tables
	symbols1 := &queryv1.TreeSymbols{
		Strings: []string{
			"",             // 0
			"binary1",      // 1
			"main",         // 2
			"/src/main.go", // 3
		},
		Mappings: []*googlev1.Mapping{
			{
				Id:          0,
				MemoryStart: 0x1000,
				MemoryLimit: 0x2000,
				Filename:    1,
			},
		},
		Functions: []*googlev1.Function{
			{
				Id:         0,
				Name:       2,
				SystemName: 2,
				Filename:   3,
				StartLine:  10,
			},
		},
		Locations: []*googlev1.Location{
			{
				Id:        0,
				Address:   0x1100,
				MappingId: 0,
				Line: []*googlev1.Line{
					{FunctionId: 0, Line: 10},
				},
			},
		},
	}

	symbols2 := &queryv1.TreeSymbols{
		Strings: []string{
			"",                // 0
			"binary2",         // 1
			"handler",         // 2
			"/src/handler.go", // 3
		},
		Mappings: []*googlev1.Mapping{
			{
				Id:          0,
				MemoryStart: 0x3000,
				MemoryLimit: 0x4000,
				Filename:    1,
			},
		},
		Functions: []*googlev1.Function{
			{
				Id:         0,
				Name:       2,
				SystemName: 2,
				Filename:   3,
				StartLine:  20,
			},
		},
		Locations: []*googlev1.Location{
			{
				Id:        0,
				Address:   0x3100,
				MappingId: 0,
				Line: []*googlev1.Line{
					{FunctionId: 0, Line: 20},
				},
			},
		},
	}

	// Merge the two symbol tables
	merger := NewSymbolMerger()

	// Add first profile and track remapping
	remap1 := merger.Add(symbols1)
	loc1Remapped := remap1(model.LocationRefName(0))

	// Add second profile and track remapping
	remap2 := merger.Add(symbols2)
	loc2Remapped := remap2(model.LocationRefName(0))

	// Build the merged result
	builder := merger.ResultBuilder()
	finalLoc1 := builder.KeepSymbol(loc1Remapped)
	finalLoc2 := builder.KeepSymbol(loc2Remapped)

	result := &queryv1.TreeSymbols{}
	builder.Build(result)

	// Validate the merged result
	t.Run("result has correct number of elements", func(t *testing.T) {
		require.Len(t, result.Locations, 2, "should have 2 locations")
		require.Len(t, result.Mappings, 2, "should have 2 mappings")
		require.Len(t, result.Functions, 2, "should have 2 functions")
		require.Len(t, result.Strings, 7, "should have 7 strings: '', 'binary1', 'main', '/src/main.go', 'binary2', 'handler', '/src/handler.go'")
	})

	t.Run("location from first profile is correct", func(t *testing.T) {
		loc := result.Locations[finalLoc1]
		require.NotNil(t, loc)
		require.Equal(t, uint64(0x1100), loc.Address, "address should match")
		require.Len(t, loc.Line, 1, "should have 1 line")

		// Get the function for this location
		funcID := loc.Line[0].FunctionId
		fn := result.Functions[funcID]
		require.NotNil(t, fn)

		// Validate function name
		funcName := result.Strings[fn.Name]
		require.Equal(t, "main", funcName, "function name should be 'main'")

		// Validate function filename
		funcFile := result.Strings[fn.Filename]
		require.Equal(t, "/src/main.go", funcFile, "function file should be '/src/main.go'")

		// Validate line number
		require.Equal(t, int64(10), loc.Line[0].Line, "line number should be 10")

		// Validate mapping
		mapping := result.Mappings[loc.MappingId]
		require.NotNil(t, mapping)
		require.Equal(t, uint64(0x1000), mapping.MemoryStart)
		mappingFile := result.Strings[mapping.Filename]
		require.Equal(t, "binary1", mappingFile, "mapping filename should be 'binary1'")
	})

	t.Run("location from second profile is correct", func(t *testing.T) {
		loc := result.Locations[finalLoc2]
		require.NotNil(t, loc)
		require.Equal(t, uint64(0x3100), loc.Address, "address should match")
		require.Len(t, loc.Line, 1, "should have 1 line")

		// Get the function for this location
		funcID := loc.Line[0].FunctionId
		fn := result.Functions[funcID]
		require.NotNil(t, fn)

		// Validate function name
		funcName := result.Strings[fn.Name]
		require.Equal(t, "handler", funcName, "function name should be 'handler'")

		// Validate function filename
		funcFile := result.Strings[fn.Filename]
		require.Equal(t, "/src/handler.go", funcFile, "function file should be '/src/handler.go'")

		// Validate line number
		require.Equal(t, int64(20), loc.Line[0].Line, "line number should be 20")

		// Validate mapping
		mapping := result.Mappings[loc.MappingId]
		require.NotNil(t, mapping)
		require.Equal(t, uint64(0x3000), mapping.MemoryStart)
		mappingFile := result.Strings[mapping.Filename]
		require.Equal(t, "binary2", mappingFile, "mapping filename should be 'binary2'")
	})

	t.Run("strings are deduplicated", func(t *testing.T) {
		// Empty string should appear only once at index 0
		require.Equal(t, "", result.Strings[0])

		// Count empty strings
		emptyCount := 0
		for _, s := range result.Strings {
			if s == "" {
				emptyCount++
			}
		}
		require.Equal(t, 1, emptyCount, "empty string should appear only once")
	})
}

// TestSymbolMerger_SharedSymbols validates that when two profiles share common symbols,
// they are properly deduplicated
func TestSymbolMerger_SharedSymbols(t *testing.T) {
	// Create two profiles that share the same binary/mapping but have different locations
	symbols1 := &queryv1.TreeSymbols{
		Strings: []string{
			"",               // 0
			"shared_binary",  // 1
			"funcA",          // 2
			"/src/shared.go", // 3
		},
		Mappings: []*googlev1.Mapping{
			{
				Id:          0,
				MemoryStart: 0x1000,
				MemoryLimit: 0x2000,
				Filename:    1,
				BuildId:     1, // Same build ID
			},
		},
		Functions: []*googlev1.Function{
			{
				Id:         0,
				Name:       2,
				SystemName: 2,
				Filename:   3,
				StartLine:  10,
			},
		},
		Locations: []*googlev1.Location{
			{
				Id:        0,
				Address:   0x1100,
				MappingId: 0,
				Line: []*googlev1.Line{
					{FunctionId: 0, Line: 10},
				},
			},
		},
	}

	symbols2 := &queryv1.TreeSymbols{
		Strings: []string{
			"",               // 0
			"shared_binary",  // 1 - SAME as symbols1
			"funcB",          // 2 - DIFFERENT
			"/src/shared.go", // 3 - SAME as symbols1
		},
		Mappings: []*googlev1.Mapping{
			{
				Id:          0,
				MemoryStart: 0x1000, // SAME as symbols1
				MemoryLimit: 0x2000, // SAME as symbols1
				Filename:    1,
				BuildId:     1, // Same build ID
			},
		},
		Functions: []*googlev1.Function{
			{
				Id:         0,
				Name:       2,
				SystemName: 2,
				Filename:   3,
				StartLine:  20, // Different line
			},
		},
		Locations: []*googlev1.Location{
			{
				Id:        0,
				Address:   0x1200, // Different address
				MappingId: 0,
				Line: []*googlev1.Line{
					{FunctionId: 0, Line: 20},
				},
			},
		},
	}

	// Merge the two symbol tables
	merger := NewSymbolMerger()

	remap1 := merger.Add(symbols1)
	loc1Remapped := remap1(model.LocationRefName(0))

	remap2 := merger.Add(symbols2)
	loc2Remapped := remap2(model.LocationRefName(0))

	// Build the merged result
	builder := merger.ResultBuilder()
	finalLoc1 := builder.KeepSymbol(loc1Remapped)
	finalLoc2 := builder.KeepSymbol(loc2Remapped)

	result := &queryv1.TreeSymbols{}
	builder.Build(result)

	t.Run("shared mapping is deduplicated", func(t *testing.T) {
		// Both locations should share the same mapping since it's identical
		require.Len(t, result.Mappings, 1, "shared mapping should be deduplicated")

		loc1 := result.Locations[finalLoc1]
		loc2 := result.Locations[finalLoc2]

		// Both should reference the same mapping
		require.Equal(t, loc1.MappingId, loc2.MappingId, "both locations should share the same mapping")

		mapping := result.Mappings[loc1.MappingId]
		require.Equal(t, uint64(0x1000), mapping.MemoryStart)
		require.Equal(t, "shared_binary", result.Strings[mapping.Filename])
	})

	t.Run("functions are not deduplicated when different", func(t *testing.T) {
		// Despite sharing the same filename, the functions are different (different names and lines)
		require.Len(t, result.Functions, 2, "should have 2 different functions")

		loc1 := result.Locations[finalLoc1]
		loc2 := result.Locations[finalLoc2]

		func1ID := loc1.Line[0].FunctionId
		func2ID := loc2.Line[0].FunctionId

		require.NotEqual(t, func1ID, func2ID, "functions should be different")

		func1 := result.Functions[func1ID]
		func2 := result.Functions[func2ID]

		require.Equal(t, "funcA", result.Strings[func1.Name])
		require.Equal(t, "funcB", result.Strings[func2.Name])
	})

	t.Run("shared strings are deduplicated", func(t *testing.T) {
		// Count occurrences of shared strings
		sharedBinaryCount := 0
		sharedFileCount := 0

		for _, s := range result.Strings {
			if s == "shared_binary" {
				sharedBinaryCount++
			}
			if s == "/src/shared.go" {
				sharedFileCount++
			}
		}

		require.Equal(t, 1, sharedBinaryCount, "shared_binary should appear only once")
		require.Equal(t, 1, sharedFileCount, "/src/shared.go should appear only once")
	})

	t.Run("locations remain distinct", func(t *testing.T) {
		require.Len(t, result.Locations, 2, "should have 2 distinct locations")

		loc1 := result.Locations[finalLoc1]
		loc2 := result.Locations[finalLoc2]

		require.NotEqual(t, loc1.Address, loc2.Address, "locations should have different addresses")
		require.Equal(t, uint64(0x1100), loc1.Address)
		require.Equal(t, uint64(0x1200), loc2.Address)
	})
}

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

// createRealisticSymbols creates a more realistic symbol table for benchmarking
// with multiple binaries, many functions, and inline frames
func createRealisticSymbols(numLocations, numMappings, numFunctions int) *Symbols {
	strings := []string{
		"", // Empty string at index 0
	}

	// Create realistic binary names
	binaries := []string{
		"/usr/bin/myapp",
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/lib/x86_64-linux-gnu/libpthread.so.0",
		"/usr/lib/x86_64-linux-gnu/libstdc++.so.6",
		"/usr/local/lib/libmylib.so",
	}

	// Create realistic file paths
	filePaths := []string{
		"/home/user/project/src/main.go",
		"/home/user/project/src/handler.go",
		"/home/user/project/src/db/connection.go",
		"/home/user/project/src/api/server.go",
		"/usr/local/go/src/runtime/proc.go",
		"/usr/local/go/src/runtime/asm_amd64.s",
		"/usr/local/go/src/net/http/server.go",
	}

	// Add binaries and file paths to strings
	strings = append(strings, binaries...)
	strings = append(strings, filePaths...)

	// Create function names
	functionNames := []string{
		"main.main",
		"main.handleRequest",
		"main.processData",
		"runtime.goexit",
		"runtime.mcall",
		"net/http.HandlerFunc.ServeHTTP",
		"net/http.(*ServeMux).ServeHTTP",
		"database/sql.(*DB).Query",
	}

	// Generate more function names if needed
	for len(functionNames) < numFunctions {
		functionNames = append(functionNames, fmt.Sprintf("pkg/module.Function%d", len(functionNames)))
	}

	strings = append(strings, functionNames...)

	// Create mappings
	mappings := make([]schemav1.InMemoryMapping, numMappings)
	baseAddr := uint64(0x400000)
	for i := 0; i < numMappings; i++ {
		mappingSize := uint64(0x100000) // 1MB per mapping
		mappings[i] = schemav1.InMemoryMapping{
			MemoryStart:     baseAddr + uint64(i)*mappingSize,
			MemoryLimit:     baseAddr + uint64(i+1)*mappingSize,
			FileOffset:      0,
			Filename:        uint32(1 + (i % len(binaries))),
			BuildId:         0,
			HasFunctions:    true,
			HasFilenames:    true,
			HasLineNumbers:  true,
			HasInlineFrames: i%3 == 0, // Some have inline frames
		}
	}

	// Create functions
	functions := make([]schemav1.InMemoryFunction, numFunctions)
	for i := 0; i < numFunctions; i++ {
		nameIdx := len(binaries) + len(filePaths) + (i % len(functionNames))
		fileIdx := len(binaries) + (i % len(filePaths))
		functions[i] = schemav1.InMemoryFunction{
			Name:       uint32(nameIdx),
			SystemName: uint32(nameIdx),
			Filename:   uint32(fileIdx),
			StartLine:  uint32(10 + (i % 100)),
		}
	}

	// Create locations with inline frames
	locations := make([]schemav1.InMemoryLocation, numLocations)
	for i := 0; i < numLocations; i++ {
		mappingIdx := i % numMappings
		baseAddr := mappings[mappingIdx].MemoryStart

		loc := schemav1.InMemoryLocation{
			MappingId: uint32(mappingIdx),
			Address:   baseAddr + uint64(i*16), // Simulate instruction addresses
		}

		// Add 1-3 inline frames for some locations
		numLines := 1
		if mappings[mappingIdx].HasInlineFrames {
			numLines = 1 + (i % 3)
		}

		for j := 0; j < numLines && j < 8; j++ {
			funcIdx := (i + j) % numFunctions
			loc.Line = append(loc.Line, schemav1.InMemoryLine{
				FunctionId: uint32(funcIdx),
				Line:       int32(50 + (i+j)%200),
			})
		}

		locations[i] = loc
	}

	return &Symbols{
		Locations: locations,
		Mappings:  mappings,
		Functions: functions,
		Strings:   strings,
	}
}

// TestSymbolMerger_FullDiagnostic provides detailed output to diagnose symbol merging issues
func TestSymbolMerger_FullDiagnostic(t *testing.T) {
	// Create two distinct profiles with clear identifiable symbols
	symbols1 := &queryv1.TreeSymbols{
		Strings:   []string{"", "app.exe", "Profile1Func", "profile1.go"},
		Mappings:  []*googlev1.Mapping{{Id: 0, MemoryStart: 0x1000, MemoryLimit: 0x2000, Filename: 1}},
		Functions: []*googlev1.Function{{Id: 0, Name: 2, SystemName: 2, Filename: 3, StartLine: 100}},
		Locations: []*googlev1.Location{{
			Id: 0, Address: 0x1234, MappingId: 0,
			Line: []*googlev1.Line{{FunctionId: 0, Line: 100}},
		}},
	}

	symbols2 := &queryv1.TreeSymbols{
		Strings:   []string{"", "app.exe", "Profile2Func", "profile2.go"},
		Mappings:  []*googlev1.Mapping{{Id: 0, MemoryStart: 0x1000, MemoryLimit: 0x2000, Filename: 1}},
		Functions: []*googlev1.Function{{Id: 0, Name: 2, SystemName: 2, Filename: 3, StartLine: 200}},
		Locations: []*googlev1.Location{{
			Id: 0, Address: 0x5678, MappingId: 0,
			Line: []*googlev1.Line{{FunctionId: 0, Line: 200}},
		}},
	}

	merger := NewSymbolMerger()

	// Add both profiles and track remapping
	remap1 := merger.Add(symbols1)
	remap2 := merger.Add(symbols2)

	loc1 := remap1(model.LocationRefName(0))
	loc2 := remap2(model.LocationRefName(0))

	builder := merger.ResultBuilder()
	finalLoc1 := builder.KeepSymbol(loc1)
	finalLoc2 := builder.KeepSymbol(loc2)

	result := &queryv1.TreeSymbols{}
	builder.Build(result)

	// Print diagnostic information
	t.Logf("=== MERGED RESULT ===")
	t.Logf("Strings: %v", result.Strings)
	t.Logf("Number of Mappings: %d", len(result.Mappings))
	t.Logf("Number of Functions: %d", len(result.Functions))
	t.Logf("Number of Locations: %d", len(result.Locations))

	t.Logf("\n=== LOCATION 1 (from Profile1) ===")
	loc1Obj := result.Locations[finalLoc1]
	t.Logf("Address: 0x%x", loc1Obj.Address)
	if len(loc1Obj.Line) > 0 {
		funcID := loc1Obj.Line[0].FunctionId
		fn := result.Functions[funcID]
		t.Logf("Function: %s (line %d)", result.Strings[fn.Name], loc1Obj.Line[0].Line)
		t.Logf("File: %s", result.Strings[fn.Filename])
	}

	t.Logf("\n=== LOCATION 2 (from Profile2) ===")
	loc2Obj := result.Locations[finalLoc2]
	t.Logf("Address: 0x%x", loc2Obj.Address)
	if len(loc2Obj.Line) > 0 {
		funcID := loc2Obj.Line[0].FunctionId
		fn := result.Functions[funcID]
		t.Logf("Function: %s (line %d)", result.Strings[fn.Name], loc2Obj.Line[0].Line)
		t.Logf("File: %s", result.Strings[fn.Filename])
	}

	// Validate that the symbols are correct
	t.Run("profile1_symbols_preserved", func(t *testing.T) {
		loc := result.Locations[finalLoc1]
		require.Equal(t, uint64(0x1234), loc.Address, "Location 1 address should be preserved")

		funcID := loc.Line[0].FunctionId
		fn := result.Functions[funcID]
		funcName := result.Strings[fn.Name]
		require.Equal(t, "Profile1Func", funcName, "Location 1 should reference Profile1Func")

		fileName := result.Strings[fn.Filename]
		require.Equal(t, "profile1.go", fileName, "Location 1 should reference profile1.go")

		require.Equal(t, int64(100), loc.Line[0].Line, "Location 1 line number should be 100")
	})

	t.Run("profile2_symbols_preserved", func(t *testing.T) {
		loc := result.Locations[finalLoc2]
		require.Equal(t, uint64(0x5678), loc.Address, "Location 2 address should be preserved")

		funcID := loc.Line[0].FunctionId
		fn := result.Functions[funcID]
		funcName := result.Strings[fn.Name]
		require.Equal(t, "Profile2Func", funcName, "Location 2 should reference Profile2Func")

		fileName := result.Strings[fn.Filename]
		require.Equal(t, "profile2.go", fileName, "Location 2 should reference profile2.go")

		require.Equal(t, int64(200), loc.Line[0].Line, "Location 2 line number should be 200")
	})
}

func BenchmarkSymbolMerger(b *testing.B) {
	// Create test data once
	ts := createTestSymbols()

	b.Run("AddSymbols", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			merger := NewSymbolMerger()
			remapFunc := merger.addSymbols(ts)
			// Exercise the remap function
			_ = remapFunc(model.LocationRefName(0))
			_ = remapFunc(model.LocationRefName(1))
		}
	})

	b.Run("AddSymbolsAndBuild", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			merger := NewSymbolMerger()
			remapFunc := merger.addSymbols(ts)
			_ = remapFunc(model.LocationRefName(0))
			_ = remapFunc(model.LocationRefName(1))

			builder := merger.ResultBuilder()
			_ = builder.KeepSymbol(model.LocationRefName(0))
			_ = builder.KeepSymbol(model.LocationRefName(1))

			result := &queryv1.TreeSymbols{}
			builder.Build(result)
		}
	})

	b.Run("AddTreeSymbols", func(b *testing.B) {
		// Pre-build TreeSymbols once
		merger := NewSymbolMerger()
		remapFunc := merger.addSymbols(ts)
		_ = remapFunc(model.LocationRefName(0))
		_ = remapFunc(model.LocationRefName(1))
		builder := merger.ResultBuilder()
		_ = builder.KeepSymbol(model.LocationRefName(0))
		_ = builder.KeepSymbol(model.LocationRefName(1))
		treeSymbols := &queryv1.TreeSymbols{}
		builder.Build(treeSymbols)

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			merger := NewSymbolMerger()
			remapFunc := merger.Add(treeSymbols)
			_ = remapFunc(model.LocationRefName(0))
			_ = remapFunc(model.LocationRefName(1))
		}
	})

	b.Run("MergeMultiple", func(b *testing.B) {
		// Pre-build TreeSymbols once
		merger := NewSymbolMerger()
		remapFunc := merger.addSymbols(ts)
		_ = remapFunc(model.LocationRefName(0))
		_ = remapFunc(model.LocationRefName(1))
		builder := merger.ResultBuilder()
		_ = builder.KeepSymbol(model.LocationRefName(0))
		_ = builder.KeepSymbol(model.LocationRefName(1))
		treeSymbols := &queryv1.TreeSymbols{}
		builder.Build(treeSymbols)

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			merger := NewSymbolMerger()
			for j := 0; j < 3; j++ {
				remapFunc := merger.Add(treeSymbols)
				_ = remapFunc(model.LocationRefName(0))
				_ = remapFunc(model.LocationRefName(1))
			}

			builder := merger.ResultBuilder()
			_ = builder.KeepSymbol(model.LocationRefName(0))
			_ = builder.KeepSymbol(model.LocationRefName(1))

			result := &queryv1.TreeSymbols{}
			builder.Build(result)
		}
	})
}

func BenchmarkSymbolMergerRealistic(b *testing.B) {
	sizes := []struct {
		name         string
		numLocations int
		numMappings  int
		numFunctions int
		numToRemap   int
		numProfiles  int
	}{
		{"Small_100loc", 100, 3, 50, 50, 3},
		{"Medium_1Kloc", 1000, 5, 200, 500, 3},
		{"Large_10Kloc", 10000, 10, 1000, 5000, 3},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			// Create realistic test data
			ts := createRealisticSymbols(sz.numLocations, sz.numMappings, sz.numFunctions)

			b.Run("AddSymbols", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					merger := NewSymbolMerger()
					remapFunc := merger.addSymbols(ts)
					// Remap a subset of locations
					for j := 0; j < sz.numToRemap; j++ {
						_ = remapFunc(model.LocationRefName(j % sz.numLocations))
					}
				}
			})

			b.Run("FullPipeline", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					merger := NewSymbolMerger()
					remapFunc := merger.addSymbols(ts)
					// Remap a subset of locations
					for j := 0; j < sz.numToRemap; j++ {
						_ = remapFunc(model.LocationRefName(j % sz.numLocations))
					}

					builder := merger.ResultBuilder()
					for j := 0; j < sz.numToRemap; j++ {
						_ = builder.KeepSymbol(model.LocationRefName(j % sz.numLocations))
					}

					result := &queryv1.TreeSymbols{}
					builder.Build(result)
				}
			})

			b.Run("MergeProfiles", func(b *testing.B) {
				// Pre-build TreeSymbols for multiple profiles
				profiles := make([]*queryv1.TreeSymbols, sz.numProfiles)
				for p := 0; p < sz.numProfiles; p++ {
					merger := NewSymbolMerger()
					remapFunc := merger.addSymbols(ts)
					remappedIndices := make([]model.LocationRefName, 0, sz.numToRemap)
					for j := 0; j < sz.numToRemap; j++ {
						idx := (j + p*100) % sz.numLocations
						remapped := remapFunc(model.LocationRefName(idx))
						remappedIndices = append(remappedIndices, remapped)
					}
					builder := merger.ResultBuilder()
					for _, remapped := range remappedIndices {
						_ = builder.KeepSymbol(remapped)
					}
					profiles[p] = &queryv1.TreeSymbols{}
					builder.Build(profiles[p])
				}

				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					merger := NewSymbolMerger()

					// Merge all profiles and track remapped locations
					allRemapped := make([]model.LocationRefName, 0)
					for _, profile := range profiles {
						remapFunc := merger.Add(profile)
						for j := 0; j < len(profile.Locations); j++ {
							remapped := remapFunc(model.LocationRefName(j))
							allRemapped = append(allRemapped, remapped)
						}
					}

					builder := merger.ResultBuilder()
					for _, remapped := range allRemapped {
						_ = builder.KeepSymbol(remapped)
					}

					result := &queryv1.TreeSymbols{}
					builder.Build(result)
				}
			})
		})
	}
}
