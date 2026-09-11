package model

import (
	"cmp"
	"context"
	"fmt"
	"math/bits"
	"slices"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// NewFunctionTableDiff joins complete function tables before selecting rows.
// Ranking uses the larger inclusive share on either side, without rounding.
func NewFunctionTableDiff(ctx context.Context, left, right *querierv1.FunctionTable, maxNodes int64) (*querierv1.FunctionTableDiff, error) {
	result := &querierv1.FunctionTableDiff{LeftTotal: left.GetTotal(), RightTotal: right.GetTotal()}
	rows := make(map[string]*querierv1.FunctionDiffRow)
	for side, table := range []*querierv1.FunctionTable{left, right} {
		if table.GetTotal() < 0 {
			return nil, fmt.Errorf("function diff requires nonnegative profile totals")
		}
		for _, function := range table.GetFunctions() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if function.Self < 0 || function.Total < 0 {
				return nil, fmt.Errorf("function diff requires nonnegative values")
			}
			row := rows[function.Name]
			if row == nil {
				row = &querierv1.FunctionDiffRow{Name: function.Name}
				rows[function.Name] = row
			}
			if side == 0 {
				row.LeftSelf += function.Self
				row.LeftTotal += function.Total
			} else {
				row.RightSelf += function.Self
				row.RightTotal += function.Total
			}
		}
	}
	result.Functions = make([]*querierv1.FunctionDiffRow, 0, len(rows))
	for _, row := range rows {
		result.Functions = append(result.Functions, row)
	}
	share := func(row *querierv1.FunctionDiffRow) (int64, int64) {
		if compareFunctionShares(row.LeftTotal, result.LeftTotal, row.RightTotal, result.RightTotal) >= 0 {
			return row.LeftTotal, result.LeftTotal
		}
		return row.RightTotal, result.RightTotal
	}
	slices.SortFunc(result.Functions, func(a, b *querierv1.FunctionDiffRow) int {
		av, at := share(a)
		bv, bt := share(b)
		if c := compareFunctionShares(bv, bt, av, at); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	if maxNodes > 0 && maxNodes < int64(len(result.Functions)) {
		result.Functions = result.Functions[:maxNodes]
	}
	return result, ctx.Err()
}

// Cross multiplication preserves ordering for int64 values beyond float64's
// integer precision. The products fit in 128 bits. Empty profiles have zero share.
func compareFunctionShares(a, totalA, b, totalB int64) int {
	if totalA == 0 {
		a, totalA = 0, 1
	}
	if totalB == 0 {
		b, totalB = 0, 1
	}
	ah, al := bits.Mul64(uint64(a), uint64(totalB))
	bh, bl := bits.Mul64(uint64(b), uint64(totalA))
	if c := cmp.Compare(ah, bh); c != 0 {
		return c
	}
	return cmp.Compare(al, bl)
}
