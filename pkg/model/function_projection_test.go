package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestFunctionTableMergeDeterministicOrder(t *testing.T) {
	merger := NewFunctionTableMerger()
	part := &typesv1.FunctionTable{
		Total: 10,
		Functions: []*typesv1.FunctionStats{{
			Name:  "B",
			Total: 5,
			Self:  5,
		}, {
			Name:  "A",
			Total: 5,
			Self:  5,
		}, {
			Name:  "main",
			Total: 10,
		}},
	}
	merger.Merge(part)
	r := merger.Table()
	require.Equal(t, int64(10), r.Total)
	require.Equal(t, int64(3), r.TotalFunctions)
	require.Equal(t, "A", r.Functions[0].Name)
	require.Equal(t, "B", r.Functions[1].Name)
	require.Equal(t, "main", r.Functions[2].Name)
	merger.Merge(part)
	require.Equal(t, int64(5), r.Functions[0].Self, "built result must remain immutable after subsequent merges")
	require.Equal(t, int64(20), merger.Table().Total)
}
