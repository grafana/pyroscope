package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

func TestHashedSliceClonesOnlyOnInsertion(t *testing.T) {
	h := newHashedSlice(func(a, b int) bool { return a == b })
	clones := 0
	clone := func(v int) int { clones++; return v }
	// Force a collision, then look up both entries again.
	require.Equal(t, int32(0), h.addWithClone(42, 1, clone))
	require.Equal(t, int32(1), h.addWithClone(42, 2, clone))
	require.Equal(t, int32(0), h.addWithClone(42, 1, clone))
	require.Equal(t, int32(1), h.addWithClone(42, 2, clone))
	require.Equal(t, 2, clones)
}

func TestSymbolMergerScratchLinesAreOwned(t *testing.T) {
	source := &Symbols{
		Strings:   []string{"", "f"},
		Functions: []schemav1.InMemoryFunction{{Name: 1}},
		Mappings:  []schemav1.InMemoryMapping{{}},
		Locations: []schemav1.InMemoryLocation{
			{Line: []schemav1.InMemoryLine{{Line: 10}, {Line: 11}}},
			{Line: []schemav1.InMemoryLine{{Line: 20}}},
			{Line: []schemav1.InMemoryLine{{Line: 30}, {Line: 31}, {Line: 32}}},
		},
	}
	leaf := NewSymbolMerger()
	remap, err := leaf.addSymbols(source, []int32{0, 1, 2})
	require.NoError(t, err)
	for i, want := range []int32{10, 20, 30} {
		require.Equal(t, want, leaf.locations.sl[int(remap(model.LocationRefName(i)))].Line[0].Line)
	}
	builder := leaf.ResultBuilder()
	for i := range source.Locations {
		builder.KeepSymbol(remap(model.LocationRefName(i)))
	}
	input := new(queryv1.TreeSymbols)
	builder.Build(input)

	aggregator := NewSymbolMerger()
	first, err := aggregator.Add(input)
	require.NoError(t, err)
	count := len(aggregator.locations.sl)
	second, err := aggregator.Add(input)
	require.NoError(t, err)
	require.Len(t, aggregator.locations.sl, count)
	for i, loc := range input.Locations {
		id := model.LocationRefName(i)
		require.Equal(t, first(id), second(id))
		stored := aggregator.locations.sl[int(first(id))]
		require.Len(t, stored.Line, len(loc.Line))
		for j, line := range loc.Line {
			require.Equal(t, int32(line.Line), stored.Line[j].Line)
			line.Line = 999
			require.NotEqual(t, int32(999), stored.Line[j].Line)
		}
	}
	// The leaf must not retain aliases to its source either.
	source.Locations[0].Line[0].Line = 999
	require.Equal(t, int32(10), leaf.locations.sl[int(remap(0))].Line[0].Line)
}
