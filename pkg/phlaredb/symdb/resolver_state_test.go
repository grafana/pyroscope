package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

func TestTreeSymbolsResetClearsCallSiteMatcher(t *testing.T) {
	symbols := &Symbols{Strings: []string{"", "f"}, Functions: []schemav1.InMemoryFunction{{Name: 1}}, Locations: []schemav1.InMemoryLocation{{Line: []schemav1.InMemoryLine{{FunctionId: 0}}}}}
	samples := schemav1.Samples{StacktraceIDs: []uint32{1}, Values: []uint64{7}}
	builder := new(treeSymbols)
	builder.init(symbols, samples, SelectStackTraces(symbols, &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{{Name: "missing"}}}), true)
	builder.InsertStacktrace(1, []int32{0})
	// Exercise reset deterministically while retaining ownership of the builder.
	builder.reset()
	require.Nil(t, builder.funcNamesMatcher)
	require.Nil(t, builder.selection)
	builder.init(symbols, samples, SelectStackTraces(symbols, nil), true)
	builder.InsertStacktrace(1, []int32{0})
	tree := model.TreeFromStacktraceTree[model.LocationRefName, model.LocationRefNameI](builder.tree, 0, func(id int32) model.LocationRefName { return model.LocationRefName(id) })
	require.Equal(t, int64(7), tree.Total(), "unfiltered reuse must not retain the previous filter")
}
