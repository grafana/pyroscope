package model

import (
	"cmp"
	"context"
	"slices"
	"sync"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// FunctionTableFromTree requires an untruncated tree. It counts a function's
// inclusive value only at its outermost occurrence on each call stack.
func FunctionTableFromTree(ctx context.Context, tree *FunctionNameTree) (*querierv1.FunctionTable, error) {
	rows := make(map[FunctionName]*querierv1.FunctionRow)
	active := make(map[FunctionName]int)
	type frame struct {
		node *node[FunctionName]
		next int
	}
	var stack []frame
	for _, root := range tree.root {
		stack = append(stack, frame{node: root, next: -1})
		for len(stack) > 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			f := &stack[len(stack)-1]
			n := f.node
			if f.next == -1 {
				row := rows[n.name]
				if row == nil {
					row = &querierv1.FunctionRow{Name: string(n.name)}
					rows[n.name] = row
				}
				row.Self += n.self
				if active[n.name] == 0 {
					row.Total += n.total
				}
				active[n.name]++
				f.next = 0
			}
			if f.next < len(n.children) {
				child := n.children[f.next]
				f.next++
				stack = append(stack, frame{node: child, next: -1})
			} else {
				active[n.name]--
				stack = stack[:len(stack)-1]
			}
		}
	}
	result := &querierv1.FunctionTable{Total: tree.Total(), Functions: make([]*querierv1.FunctionRow, 0, len(rows))}
	for _, row := range rows {
		result.Functions = append(result.Functions, row)
	}
	return result, ctx.Err()
}

// FunctionTableMerger sums complete function tables. It must not receive
// partial top-N results: their missing rows cannot be recovered by merging.
type FunctionTableMerger struct {
	mu    sync.Mutex
	rows  map[string]*querierv1.FunctionRow
	total int64
}

func (m *FunctionTableMerger) Merge(table *querierv1.FunctionTable) {
	if table == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = make(map[string]*querierv1.FunctionRow, len(table.Functions))
	}
	m.total += table.Total
	for _, row := range table.Functions {
		v := m.rows[row.Name]
		if v == nil {
			v = &querierv1.FunctionRow{Name: row.Name}
			m.rows[row.Name] = v
		}
		v.Self += row.Self
		v.Total += row.Total
	}
}

// Table returns every row, without ordering or limiting intermediate results.
func (m *FunctionTableMerger) Table() *querierv1.FunctionTable {
	m.mu.Lock()
	defer m.mu.Unlock()
	table := &querierv1.FunctionTable{Total: m.total, Functions: make([]*querierv1.FunctionRow, 0, len(m.rows))}
	for _, row := range m.rows {
		table.Functions = append(table.Functions, row.CloneVT())
	}
	return table
}

// LimitFunctionTable sorts and limits the final result, preserving its total.
// A nonpositive limit returns every function.
func LimitFunctionTable(table *querierv1.FunctionTable, limit int64) {
	slices.SortFunc(table.Functions, func(a, b *querierv1.FunctionRow) int {
		if c := cmp.Compare(b.Self, a.Self); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	if limit > 0 && limit < int64(len(table.Functions)) {
		table.Functions = table.Functions[:limit]
	}
}
