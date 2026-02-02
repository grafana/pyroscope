package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

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
			require.Equal(t, model.LocationRefName(locID), f(model.LocationRefName(locID)))
		}
	}

	adder := merger.addSymbols(ts, locIDs)
	keepAll(adder)

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
