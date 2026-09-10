package model

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestFunctionTableDiff_RankingAndJoin(t *testing.T) {
	t.Parallel()
	left := &querierv1.FunctionTable{Total: 100, Functions: []*querierv1.FunctionRow{
		{Name: "baseline-hot", Self: 20, Total: 80},
		{Name: "comparison-hot", Self: 1, Total: 10},
		{Name: "large-raw", Self: 79, Total: 79},
	}}
	right := &querierv1.FunctionTable{Total: 1000, Functions: []*querierv1.FunctionRow{
		{Name: "comparison-hot", Self: 1, Total: 900},
		{Name: "large-raw", Self: 900, Total: 900},
		{Name: "new", Self: 99, Total: 99},
	}}
	for _, limit := range []int64{0, -1, 1, 10} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			diff, err := NewFunctionTableDiff(context.Background(), left, right, limit)
			require.NoError(t, err)
			require.Equal(t, int64(100), diff.LeftTotal)
			require.Equal(t, int64(1000), diff.RightTotal)
			want := []*querierv1.FunctionDiffRow{
				{Name: "comparison-hot", LeftSelf: 1, LeftTotal: 10, RightSelf: 1, RightTotal: 900},
				{Name: "large-raw", LeftSelf: 79, LeftTotal: 79, RightSelf: 900, RightTotal: 900},
				{Name: "baseline-hot", LeftSelf: 20, LeftTotal: 80},
				{Name: "new", RightSelf: 99, RightTotal: 99},
			}
			if limit == 1 {
				want = want[:1]
			}
			require.Equal(t, want, diff.Functions)
		})
	}
	// Source reports remain complete and in their original order.
	require.Len(t, left.Functions, 3)
	require.Equal(t, "baseline-hot", left.Functions[0].Name)
}

func TestFunctionTableDiff_ExactRanking(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		left, right *querierv1.FunctionTable
		first       string
	}{
		{
			name:  "normalize by each profile total",
			left:  &querierv1.FunctionTable{Total: 100, Functions: []*querierv1.FunctionRow{{Name: "left", Total: 50}}},
			right: &querierv1.FunctionTable{Total: 1000, Functions: []*querierv1.FunctionRow{{Name: "right", Total: 400}}},
			first: "left",
		},
		{
			name:  "equal fractions tie by name",
			left:  &querierv1.FunctionTable{Total: 3, Functions: []*querierv1.FunctionRow{{Name: "z", Total: 1}}},
			right: &querierv1.FunctionTable{Total: 6, Functions: []*querierv1.FunctionRow{{Name: "a", Total: 2}}},
			first: "a",
		},
		{
			name: "beyond float precision and int64 products",
			left: &querierv1.FunctionTable{Total: math.MaxInt64, Functions: []*querierv1.FunctionRow{
				{Name: "a", Total: 1 << 53}, {Name: "z", Total: 1<<53 + 1},
			}},
			first: "z",
		},
		{
			name:  "empty baseline",
			right: &querierv1.FunctionTable{Total: 10, Functions: []*querierv1.FunctionRow{{Name: "new", Self: 10, Total: 10}}},
			first: "new",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diff, err := NewFunctionTableDiff(context.Background(), tc.left, tc.right, 1)
			require.NoError(t, err)
			require.Len(t, diff.Functions, 1)
			require.Equal(t, tc.first, diff.Functions[0].Name)
		})
	}
	diff, err := NewFunctionTableDiff(context.Background(), nil, nil, 10)
	require.NoError(t, err)
	require.Empty(t, diff.Functions)
	require.Zero(t, diff.LeftTotal)
	require.Zero(t, diff.RightTotal)
}

func TestFunctionTableDiff_GrafanaParity(t *testing.T) {
	t.Parallel()
	left := new(FunctionNameTree)
	left.InsertStack(10, "root", "f", "f")
	left.InsertStack(10, "root", "f")
	left.InsertStack(80, "root", "other")
	right := new(FunctionNameTree)
	right.InsertStack(60, "root", "f", "f")
	right.InsertStack(140, "root", "other")
	l, err := FunctionTableFromTree(context.Background(), left)
	require.NoError(t, err)
	r, err := FunctionTableFromTree(context.Background(), right)
	require.NoError(t, err)
	diff, err := NewFunctionTableDiff(context.Background(), l, r, 0)
	require.NoError(t, err)
	var f *querierv1.FunctionDiffRow
	for _, row := range diff.Functions {
		if row.Name == "f" {
			f = row
		}
	}
	require.Equal(t, &querierv1.FunctionDiffRow{Name: "f", LeftSelf: 20, LeftTotal: 20, RightSelf: 60, RightTotal: 60}, f)
	// Grafana TopTable rounds inclusive profile shares to two decimal places
	// before computing relative change. It does not normalize by elapsed time.
	for _, tc := range []struct {
		name                           string
		left, right, leftSum, rightSum int64
		want                           float64
	}{
		{"20 percent to 30 percent", f.LeftTotal, f.RightTotal, diff.LeftTotal, diff.RightTotal, 50},
		{"new", 0, 10, 100, 100, math.Inf(1)},
		{"removed", 10, 0, 100, 100, -100},
		{"both zero", 0, 0, 100, 100, math.NaN()},
		{"empty baseline", 0, 10, 0, 100, math.NaN()},
		{"rounding to zero", 4, 6, 100000, 100000, math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewFunctionTableDiff(context.Background(),
				&querierv1.FunctionTable{Total: tc.leftSum, Functions: []*querierv1.FunctionRow{{Name: "f", Total: tc.left}}},
				&querierv1.FunctionTable{Total: tc.rightSum, Functions: []*querierv1.FunctionRow{{Name: "f", Total: tc.right}}}, 1)
			require.NoError(t, err)
			row := result.Functions[0]
			baseline := math.Round(10000*float64(row.LeftTotal)/float64(result.LeftTotal)) / 100
			comparison := math.Round(10000*float64(row.RightTotal)/float64(result.RightTotal)) / 100
			change := 100 * (comparison - baseline) / baseline
			if math.IsNaN(tc.want) {
				require.True(t, math.IsNaN(change))
			} else {
				require.Equal(t, tc.want, change)
			}
		})
	}
}

func TestFunctionTableDiff_Errors(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewFunctionTableDiff(ctx, nil, nil, 0)
	require.ErrorIs(t, err, context.Canceled)
	for _, table := range []*querierv1.FunctionTable{
		{Total: -1},
		{Total: 1, Functions: []*querierv1.FunctionRow{{Self: -1}}},
		{Total: 1, Functions: []*querierv1.FunctionRow{{Total: -1}}},
	} {
		_, err := NewFunctionTableDiff(context.Background(), table, nil, 0)
		require.Error(t, err)
	}
}

func BenchmarkFunctionTableDiff(b *testing.B) {
	left, right := new(querierv1.FunctionTable), new(querierv1.FunctionTable)
	for i := range 10000 {
		left.Functions = append(left.Functions, &querierv1.FunctionRow{Name: fmt.Sprint(i), Self: int64(i), Total: int64(i)})
		right.Functions = append(right.Functions, &querierv1.FunctionRow{Name: fmt.Sprint(i + 5000), Self: int64(i), Total: int64(i)})
		left.Total += int64(i)
		right.Total += int64(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := NewFunctionTableDiff(context.Background(), left, right, 100); err != nil {
			b.Fatal(err)
		}
	}
}
