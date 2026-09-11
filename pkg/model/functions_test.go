package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestFunctionTableFromTree(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(3, "root", "a", "a")
	tree.InsertStack(5, "root", "a", "b", "a")
	tree.InsertStack(7, "root", "b", "a")
	tree.InsertStack(2, "root", "a", "b")
	tree.InsertStack(1, "other") // A real function named other must be retained.
	table, err := FunctionTableFromTree(context.Background(), tree)
	require.NoError(t, err)
	LimitFunctionTable(table, -1)
	require.Equal(t, int64(18), table.Total)
	require.Equal(t, []*querierv1.FunctionRow{
		{Name: "a", Self: 15, Total: 17},
		{Name: "b", Self: 2, Total: 14},
		{Name: "other", Self: 1, Total: 1},
		{Name: "root", Total: 17},
	}, table.Functions)
	LimitFunctionTable(table, 1)
	require.Len(t, table.Functions, 1)
	require.Equal(t, int64(18), table.Total)
}

func TestFunctionTableFromTree_EmptyAndCanceled(t *testing.T) {
	t.Parallel()
	table, err := FunctionTableFromTree(context.Background(), new(FunctionNameTree))
	require.NoError(t, err)
	require.Empty(t, table.Functions)
	require.Zero(t, table.Total)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = FunctionTableFromTree(ctx, new(FunctionNameTree))
	require.ErrorIs(t, err, context.Canceled)
}

func TestFunctionTableMerger_GlobalRanking(t *testing.T) {
	t.Parallel()
	var merger FunctionTableMerger
	for _, name := range []string{"local-a", "local-b"} {
		merger.Merge(&querierv1.FunctionTable{Total: 19, Functions: []*querierv1.FunctionRow{
			{Name: name, Self: 10, Total: 10},
			{Name: "global", Self: 9, Total: 9},
		}})
	}
	table := merger.Table()
	LimitFunctionTable(table, 1)
	require.Equal(t, int64(38), table.Total)
	require.Equal(t, []*querierv1.FunctionRow{{Name: "global", Self: 18, Total: 18}}, table.Functions)
	// Sorting and limiting a result does not change the merger's retained rows.
	table = merger.Table()
	LimitFunctionTable(table, -1)
	require.Equal(t, []string{"global", "local-a", "local-b"}, []string{
		table.Functions[0].Name, table.Functions[1].Name, table.Functions[2].Name,
	})
}

func BenchmarkFunctionTableFromTree(b *testing.B) {
	tree := new(FunctionNameTree)
	for i := range 10000 {
		tree.InsertStack(1, "root", FunctionName(fmt.Sprintf("caller%d", i)), FunctionName(fmt.Sprintf("leaf%d", i%100)))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := FunctionTableFromTree(context.Background(), tree); err != nil {
			b.Fatal(err)
		}
	}
}
