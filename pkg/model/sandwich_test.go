package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// half resolves names through the report's table, so the assertions below can
// still read as a tree walk.
type half struct {
	t     *testing.T
	names []string
	node  *querierv1.SandwichNode
}

func callers(t *testing.T, r *querierv1.SandwichReport) half {
	return half{t: t, names: r.Names, node: r.Callers}
}

func callees(t *testing.T, r *querierv1.SandwichReport) half {
	return half{t: t, names: r.Names, node: r.Callees}
}

func (h half) child(name string) half {
	h.t.Helper()
	require.NotNil(h.t, h.node, "no parent to look for %q under", name)
	for _, c := range h.node.Children {
		if nameAt(h.names, c.NameIndex) == FunctionName(name) {
			return half{t: h.t, names: h.names, node: c}
		}
	}
	require.Failf(h.t, "missing child", "%q has no child %q, only %v",
		nameAt(h.names, h.node.NameIndex), name, h.childNames())
	return half{}
}

func (h half) childNames() []string {
	names := make([]string, 0, len(h.node.Children))
	for _, c := range h.node.Children {
		names = append(names, string(nameAt(h.names, c.NameIndex)))
	}
	return names
}

func (h half) total() int64    { return h.node.Total }
func (h half) self() int64     { return h.node.Self }
func (h half) truncated() bool { return h.node.Truncated }
func (h half) children() int   { return len(h.node.Children) }
func (h half) name() string    { return string(nameAt(h.names, h.node.NameIndex)) }

func TestSandwichFromTree(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(3, "root", "a", "f", "x")
	tree.InsertStack(5, "root", "a", "f", "y") // same call site as above, so it merges
	tree.InsertStack(7, "root", "b", "f", "x")

	r, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)

	// Counted once per sample, so it can be compared against the function table.
	require.Equal(t, int64(15), r.Total)
	require.Zero(t, r.Self)

	// Callers: a contributed 8 to f, b contributed 7, and each reaches root.
	require.Equal(t, int64(15), r.Callers.Total)
	require.Equal(t, []string{"a", "b"}, callers(t, r).childNames()) // total desc: a gave 8, b gave 7
	require.Equal(t, int64(8), callers(t, r).child("a").total())
	require.Equal(t, int64(7), callers(t, r).child("b").total())
	require.Equal(t, int64(8), callers(t, r).child("a").child("root").total())
	require.Equal(t, int64(7), callers(t, r).child("b").child("root").total())

	// A caller node carries what it gave to f, not its own total: a's own total
	// in the tree is 8 and b's is 7, and here they are the same numbers only
	// because everything under them reaches f.
	require.Zero(t, callers(t, r).child("a").child("root").children())

	// Callees: x was reached through both call sites, y through one.
	require.Equal(t, int64(15), r.Callees.Total)
	require.Equal(t, int64(10), callees(t, r).child("x").total())
	require.Equal(t, int64(10), callees(t, r).child("x").self())
	require.Equal(t, int64(5), callees(t, r).child("y").total())
}

// A recursive function is present on both sides at several depths. That shape is
// intended: it is what @grafana/flamegraph renders today, and what makes the
// sandwich of a recursive function look the way it does.
func TestSandwichFromTree_Recursion(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(2, "root", "f", "f", "f")

	r, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)

	// Counted once per sample: the three occurrences are the same 2 samples.
	require.Equal(t, int64(2), r.Total)
	require.Equal(t, int64(2), r.Self)

	// Both halves sum the occurrences, so their roots exceed Total. This is the
	// client's behaviour, kept deliberately; Total is the undistorted number.
	require.Equal(t, int64(6), r.Callers.Total)
	require.Equal(t, int64(6), r.Callees.Total)

	// Callers staircase: f above f above f, then root at each depth.
	require.Equal(t, int64(4), callers(t, r).child("f").total())
	require.Equal(t, int64(2), callers(t, r).child("root").total())
	require.Equal(t, int64(2), callers(t, r).child("f").child("f").total())
	require.Equal(t, int64(2), callers(t, r).child("f").child("root").total())
	require.Equal(t, int64(2), callers(t, r).child("f").child("f").child("root").total())

	// Callees staircase: the subtree of each occurrence, merged.
	require.Equal(t, int64(4), callees(t, r).child("f").total())
	require.Equal(t, int64(2), callees(t, r).child("f").child("f").total())
}

func TestSandwichFromTree_RecursionTotalMatchesFunctionTable(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(2, "root", "f", "f", "f")
	tree.InsertStack(3, "root", "g", "f")

	r, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)
	table, err := FunctionTableFromTree(context.Background(), tree)
	require.NoError(t, err)
	LimitFunctionTable(table, -1)

	var row *querierv1.FunctionRow
	for _, fr := range table.Functions {
		if fr.Name == "f" {
			row = fr
		}
	}
	require.NotNil(t, row)
	// The whole reason the sandwich carries a counted-once total: option E
	// compares these two to decide whether a sandwich is complete.
	require.Equal(t, row.Total, r.Total)
	require.Equal(t, row.Self, r.Self)
}

func TestSandwichFromTree_NoOccurrences(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(1, "root", "a")

	r, err := SandwichFromTree(context.Background(), tree, "missing")
	require.NoError(t, err)
	require.Zero(t, r.Total)
	require.Zero(t, r.Self)
	require.Zero(t, r.Callers.Total)
	require.Zero(t, callers(t, r).children())
	require.Zero(t, callees(t, r).children())
}

func TestSandwichFromTree_Canceled(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(1, "root", "f")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SandwichFromTree(ctx, tree, "f")
	require.ErrorIs(t, err, context.Canceled)
}

func TestSandwichMerger(t *testing.T) {
	t.Parallel()
	var merger SandwichMerger

	for _, caller := range []FunctionName{"a", "b"} {
		tree := new(FunctionNameTree)
		tree.InsertStack(4, "root", caller, "f", "shared")
		r, err := SandwichFromTree(context.Background(), tree, "f")
		require.NoError(t, err)
		merger.Merge(r)
	}

	merged := merger.Sandwich(-1)
	require.Equal(t, int64(8), merged.Total)
	require.Equal(t, int64(8), merged.Callers.Total)
	require.Equal(t, int64(4), callers(t, merged).child("a").total())
	require.Equal(t, int64(4), callers(t, merged).child("b").total())
	// The callee is the same function on both shards, so it merges into one node.
	require.Equal(t, 1, callees(t, merged).children())
	require.Equal(t, int64(8), callees(t, merged).child("shared").total())
}

func TestSandwichMerger_Empty(t *testing.T) {
	t.Parallel()
	var merger SandwichMerger
	merger.Merge(nil)
	r := merger.Sandwich(16)
	require.Zero(t, r.Total)
	require.Nil(t, r.Callers)
}

func TestSandwichTruncation(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	// One fat callee and several thin ones, so truncation has an obvious choice.
	tree.InsertStack(100, "root", "f", "fat")
	for _, thin := range []FunctionName{"t1", "t2", "t3", "t4"} {
		tree.InsertStack(1, "root", "f", thin)
	}

	r, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)
	require.Equal(t, 5, callees(t, r).children())

	var merger SandwichMerger
	merger.Merge(r)
	// Root plus the fat callee plus the stand-in.
	truncated := merger.Sandwich(2)

	require.Equal(t, int64(100), callees(t, truncated).child("fat").total())
	other := callees(t, truncated).child(string(OtherFunctionName))
	require.True(t, other.truncated(), "the stand-in must be marked so a pane can label it")
	require.Equal(t, int64(4), other.total(), "the stand-in carries what was dropped")
	require.Equal(t, int64(4), other.self())
	// Truncating a half must not change what the profile says is in f.
	require.Equal(t, int64(104), truncated.Total)
	require.Equal(t, int64(104), truncated.Callees.Total)
}

func TestSandwichTruncation_RealOtherFunction(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	// A callee genuinely named 'other', big enough to survive, must not be
	// reported as a truncation stand-in.
	tree.InsertStack(100, "root", "f", "other")
	tree.InsertStack(1, "root", "f", "thin")

	r, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)
	require.False(t, callees(t, r).child("other").truncated())

	var merger SandwichMerger
	merger.Merge(r)
	truncated := merger.Sandwich(2)
	real := callees(t, truncated).child("other")
	require.Equal(t, int64(101), real.total(), "the dropped mass folds into the real node, as tree truncation does")
	require.True(t, real.truncated(), "and it now also stands in for what was dropped")
}

func TestSandwichNameTable(t *testing.T) {
	t.Parallel()
	tree := new(FunctionNameTree)
	tree.InsertStack(3, "root", "a", "f", "shared")
	tree.InsertStack(5, "root", "b", "f", "shared")
	tree.InsertStack(2, "root", "a", "f", "f") // recursion, so f spans both halves

	r, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)

	seen := make(map[string]int)
	for i, n := range r.Names {
		seen[n]++
		require.Equal(t, 1, seen[n], "name %q interned twice, at index %d", n, i)
	}
	// One entry per distinct function across both halves, not per node.
	require.ElementsMatch(t, []string{"f", "a", "b", "root", "shared"}, r.Names)

	var walk func(n *querierv1.SandwichNode)
	walk = func(n *querierv1.SandwichNode) {
		require.GreaterOrEqual(t, n.NameIndex, int32(0))
		require.Less(t, int(n.NameIndex), len(r.Names), "index out of the table")
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(r.Callers)
	walk(r.Callees)

	// The halves share the table, so both roots point at the same entry.
	require.Equal(t, r.Callers.NameIndex, r.Callees.NameIndex)
	require.Equal(t, "f", string(nameAt(r.Names, r.Callers.NameIndex)))
}

func TestSandwichNameAtOutOfRange(t *testing.T) {
	t.Parallel()
	// A malformed response from another process must not panic the merger.
	var merger SandwichMerger
	merger.Merge(&querierv1.SandwichReport{
		Names:   []string{"f"},
		Callers: &querierv1.SandwichNode{NameIndex: 0, Total: 5, Children: []*querierv1.SandwichNode{{NameIndex: 9, Total: 5}}},
		Total:   5,
	})
	r := merger.Sandwich(-1)
	require.Equal(t, int64(5), r.Total)
	require.Equal(t, 1, callers(t, r).children())
	require.Equal(t, "", callers(t, r).child("").name())
}

func TestSandwichMerger_EmptyDatasetHasNoPhantomRoot(t *testing.T) {
	t.Parallel()
	var merger SandwichMerger
	// What a dataset with no matching samples reports.
	merger.Merge(&querierv1.SandwichReport{})
	r := merger.Sandwich(16384)
	require.Nil(t, r.Callers)
	require.Nil(t, r.Callees)
	require.Empty(t, r.Names, "an empty sandwich must not name a function it never saw")
	require.Zero(t, r.Total)

	// A real report after an empty one still merges normally.
	tree := new(FunctionNameTree)
	tree.InsertStack(4, "root", "a", "f")
	real, err := SandwichFromTree(context.Background(), tree, "f")
	require.NoError(t, err)
	merger.Merge(real)
	r = merger.Sandwich(16384)
	require.Equal(t, int64(4), r.Total)
	require.Equal(t, "f", callers(t, r).name())
	require.Equal(t, int64(4), callers(t, r).child("a").total())
}
