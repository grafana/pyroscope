package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

func TestSymbolMergerDoesNotMutateSourceLines(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"", "f"},
		Functions: []schemav1.InMemoryFunction{{Name: 1}},
		Mappings:  []schemav1.InMemoryMapping{{}},
		Locations: []schemav1.InMemoryLocation{{Line: []schemav1.InMemoryLine{{FunctionId: 0, Line: 42}}}},
	}
	before := append([]schemav1.InMemoryLine(nil), symbols.Locations[0].Line...)
	_, err := NewSymbolMerger().addSymbols(symbols, []int32{0})
	require.NoError(t, err)
	require.Equal(t, before, symbols.Locations[0].Line)
}

func TestSymbolResultBuilderDoesNotMutateMergerLines(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"", "unused", "kept"},
		Functions: []schemav1.InMemoryFunction{{}, {Name: 1}, {Name: 2}},
		Mappings:  []schemav1.InMemoryMapping{{}},
		Locations: []schemav1.InMemoryLocation{
			{},
			{Line: []schemav1.InMemoryLine{{FunctionId: 1, Line: 10}}},
			{Line: []schemav1.InMemoryLine{{FunctionId: 2, Line: 42}}},
		},
	}
	merger := NewSymbolMerger()
	remap, err := merger.addSymbols(symbols, []int32{0, 1, 2})
	require.NoError(t, err)
	kept := remap(model.LocationRefName(2))
	before := append([]schemav1.InMemoryLine(nil), merger.locations.sl[int(kept)].Line...)

	// Excluding the first function forces the kept function's ID to change.
	build := func() *queryv1.TreeSymbols {
		builder := merger.ResultBuilder()
		builder.KeepSymbol(kept)
		result := new(queryv1.TreeSymbols)
		builder.Build(result)
		return result
	}
	first := build()
	line := first.Locations[1].Line[0]
	require.NotEqual(t, uint64(before[0].FunctionId), line.FunctionId)
	require.Equal(t, "kept", first.Strings[first.Functions[line.FunctionId].Name])
	require.Equal(t, before, merger.locations.sl[int(kept)].Line,
		"building a subset must not remap the merger's stored function IDs")
	require.Equal(t, first, build(), "repeated builds must produce identical symbols")
}
