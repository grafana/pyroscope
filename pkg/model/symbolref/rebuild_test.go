package symbolref_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
)

// functionStacks walks every stack in tree and sums self values keyed by
// the root-to-leaf sequence of names joined with "/".
func functionStacks(t *testing.T, tree *model.FunctionNameTree) map[string]int64 {
	t.Helper()
	out := make(map[string]int64)
	tree.IterateStacks(func(_ model.FunctionName, self int64, stack []model.FunctionName) {
		slices.Reverse(stack)
		names := make([]string, len(stack))
		for i, n := range stack {
			names[i] = string(n)
		}
		out[strings.Join(names, "/")] += self
	})
	return out
}

// TestRebuildInlineChainExpansionOrder verifies one unresolved ref resolving
// to multiple inlined frames expands into that many tree levels, in exactly
// the order resolve returned them.
func TestRebuildInlineChainExpansionOrder(t *testing.T) {
	table := symbolref.NewTable()
	loc := table.InternUnresolved("build1", "bin1", 0x1000)

	tree := new(model.LocationRefNameTree)
	tree.InsertStack(5, loc)

	rb := table.ResultBuilder()
	treeBytes := tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	resolve := func(buildID string, addr uint64) []symbolref.Frame {
		require.Equal(t, "build1", buildID)
		require.Equal(t, uint64(0x1000), addr)
		return []symbolref.Frame{{Name: "outer"}, {Name: "inlined_middle"}, {Name: "inner"}}
	}

	out, err := symbolref.Rebuild(treeBytes, pb, resolve, 0)
	require.NoError(t, err)

	got, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](out)
	require.NoError(t, err)

	stacks := functionStacks(t, got)
	require.Equal(t, int64(5), stacks["outer/inlined_middle/inner"])
}

// TestRebuildDuplicateNodeMerge verifies two different unresolved addresses
// that resolve to the same function name, at the same tree position, merge
// into one node with combined weight.
func TestRebuildDuplicateNodeMerge(t *testing.T) {
	table := symbolref.NewTable()
	parent := table.InternName("parent")
	loc1 := table.InternUnresolved("build1", "bin1", 0x1000)
	loc2 := table.InternUnresolved("build1", "bin1", 0x2000)

	tree := new(model.LocationRefNameTree)
	tree.InsertStack(3, parent, loc1)
	tree.InsertStack(4, parent, loc2)

	rb := table.ResultBuilder()
	treeBytes := tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	resolve := func(buildID string, addr uint64) []symbolref.Frame {
		return []symbolref.Frame{{Name: "malloc"}}
	}

	out, err := symbolref.Rebuild(treeBytes, pb, resolve, 0)
	require.NoError(t, err)

	got, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](out)
	require.NoError(t, err)

	stacks := functionStacks(t, got)
	require.Len(t, stacks, 1)
	require.Equal(t, int64(7), stacks["parent/malloc"])
}

// TestRebuildFallbackRendering verifies an unresolved ref whose resolve
// callback returns nil renders exactly the same fallback string
// createFallbackSymbol produces (pkg/symbolizer/symbolizer.go), including
// the empty-binaryName substitution to "unknown".
func TestRebuildFallbackRendering(t *testing.T) {
	for _, tc := range []struct {
		name       string
		binaryName string
		want       string
	}{
		{"with binary name", "myapp", "myapp!0x1a2b"},
		{"empty binary name", "", "unknown!0x1a2b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := symbolref.NewTable()
			loc := table.InternUnresolved("deadbeef", tc.binaryName, 0x1a2b)

			tree := new(model.LocationRefNameTree)
			tree.InsertStack(1, loc)

			rb := table.ResultBuilder()
			treeBytes := tree.Bytes(0, rb.KeepRef)
			pb := new(queryv1.SymbolRefTable)
			rb.Build(pb)

			resolve := func(buildID string, addr uint64) []symbolref.Frame { return nil }

			out, err := symbolref.Rebuild(treeBytes, pb, resolve, 0)
			require.NoError(t, err)

			got, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](out)
			require.NoError(t, err)

			stacks := functionStacks(t, got)
			require.Equal(t, int64(1), stacks[tc.want])
		})
	}
}

// TestRebuildTruncationOnce verifies applying maxNodes truncation only once,
// after resolution, retains a node that per-partial (pre-resolution)
// truncation would have dropped.
func TestRebuildTruncationOnce(t *testing.T) {
	const maxNodes = 2

	buildPartial1 := func() (*symbolref.Table, *model.LocationRefNameTree) {
		table := symbolref.NewTable()
		big1a := table.InternName("big1a")
		big1b := table.InternName("big1b")
		addrA := table.InternUnresolved("buildX", "binX", 0xA)
		tree := new(model.LocationRefNameTree)
		tree.InsertStack(10, big1a)
		tree.InsertStack(9, big1b)
		tree.InsertStack(6, addrA)
		return table, tree
	}
	buildPartial2 := func() (*symbolref.Table, *model.LocationRefNameTree) {
		table := symbolref.NewTable()
		big2a := table.InternName("big2a")
		big2b := table.InternName("big2b")
		addrB := table.InternUnresolved("buildX", "binX", 0xB)
		tree := new(model.LocationRefNameTree)
		tree.InsertStack(11, big2a)
		tree.InsertStack(8, big2b)
		tree.InsertStack(7, addrB)
		return table, tree
	}

	resolve := func(buildID string, addr uint64) []symbolref.Frame {
		return []symbolref.Frame{{Name: "shared_symbol"}}
	}

	// rebuildMerged marshals each partial at partialTreeMaxNodes, merges
	// them, and rebuilds at finalMaxNodes — the only difference between the
	// deferred-truncation path and the pre-truncated baseline below is
	// partialTreeMaxNodes.
	rebuildMerged := func(t *testing.T, partialTreeMaxNodes int64) map[string]int64 {
		table1, tree1 := buildPartial1()
		table2, tree2 := buildPartial2()

		rb1 := table1.ResultBuilder()
		treeBytes1 := tree1.Bytes(partialTreeMaxNodes, rb1.KeepRef)
		pb1 := new(queryv1.SymbolRefTable)
		rb1.Build(pb1)

		rb2 := table2.ResultBuilder()
		treeBytes2 := tree2.Bytes(partialTreeMaxNodes, rb2.KeepRef)
		pb2 := new(queryv1.SymbolRefTable)
		rb2.Build(pb2)

		dst := symbolref.NewTable()
		merger := model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()
		remap1, err := dst.Add(pb1)
		require.NoError(t, err)
		require.NoError(t, merger.MergeTreeBytes(treeBytes1, model.WithTreeMergeFormatNodeNames(remap1)))
		remap2, err := dst.Add(pb2)
		require.NoError(t, err)
		require.NoError(t, merger.MergeTreeBytes(treeBytes2, model.WithTreeMergeFormatNodeNames(remap2)))

		finalRB := dst.ResultBuilder()
		mergedTreeBytes := merger.Tree().Bytes(0, finalRB.KeepRef)
		mergedPB := new(queryv1.SymbolRefTable)
		finalRB.Build(mergedPB)

		out, err := symbolref.Rebuild(mergedTreeBytes, mergedPB, resolve, maxNodes)
		require.NoError(t, err)
		got, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](out)
		require.NoError(t, err)
		return functionStacks(t, got)
	}

	// Deferred-truncation path: both partials marshal untruncated (since
	// HasUnresolved() is true for both, real truncation would be unsound),
	// merged untruncated, resolved, and truncated exactly once at the end.
	deferredStacks := rebuildMerged(t, 0)
	require.Equal(t, int64(13), deferredStacks["shared_symbol"],
		"combined weight from both partials should survive maxNodes=%d", maxNodes)

	// Pre-truncated baseline: each partial truncated individually at
	// maxNodes before merging — deliberately unsound, not part of the
	// package's API, kept here only to demonstrate what deferred truncation
	// avoids.
	baselineStacks := rebuildMerged(t, maxNodes)
	require.NotContains(t, baselineStacks, "shared_symbol")
}

// TestRebuildOtherNodePreservation verifies a partial's own pre-existing
// truncation "other" node survives merge and Rebuild unchanged, and that
// additional truncation mass Rebuild's own final maxNodes pass produces is
// accounted for alongside it, not in place of it.
func TestRebuildOtherNodePreservation(t *testing.T) {
	const (
		partialMaxNodes = 2
		finalMaxNodes   = 2
	)

	// Partial A: a plain FunctionNameTree, pre-truncated normally (it has no
	// unresolved entries, so ordinary truncation is sound), producing a
	// nonzero pre-existing "other" mass.
	plainTree := new(model.FunctionNameTree)
	plainTree.InsertStack(10, model.FunctionName("keep1"))
	plainTree.InsertStack(9, model.FunctionName("keep2"))
	plainTree.InsertStack(6, model.FunctionName("small_dropped"))
	plainTreeBytes := plainTree.Bytes(partialMaxNodes, nil)

	dst := symbolref.NewTable()
	merger := model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()
	merger.MergeTree(absorbPlainTree(t, dst, plainTreeBytes))

	// Partial B: a genuine unresolved partial, plus a large resolved node
	// so the merged tree has enough nodes for a small final maxNodes to
	// force additional truncation.
	tableB := symbolref.NewTable()
	keep3 := tableB.InternName("keep3")
	loc := tableB.InternUnresolved("buildX", "binX", 0x1)
	treeB := new(model.LocationRefNameTree)
	treeB.InsertStack(20, keep3)
	treeB.InsertStack(2, loc)
	rbB := tableB.ResultBuilder()
	treeBytesB := treeB.Bytes(0, rbB.KeepRef)
	pbB := new(queryv1.SymbolRefTable)
	rbB.Build(pbB)

	remapB, err := dst.Add(pbB)
	require.NoError(t, err)
	require.NoError(t, merger.MergeTreeBytes(treeBytesB, model.WithTreeMergeFormatNodeNames(remapB)))

	finalRB := dst.ResultBuilder()
	mergedTreeBytes := merger.Tree().Bytes(0, finalRB.KeepRef)
	mergedPB := new(queryv1.SymbolRefTable)
	finalRB.Build(mergedPB)

	resolve := func(buildID string, addr uint64) []symbolref.Frame {
		return []symbolref.Frame{{Name: "resolved_small"}}
	}

	out, err := symbolref.Rebuild(mergedTreeBytes, mergedPB, resolve, finalMaxNodes)
	require.NoError(t, err)

	got, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](out)
	require.NoError(t, err)

	stacks := functionStacks(t, got)
	require.GreaterOrEqual(t, stacks["other"], int64(6),
		"other mass must include at least the 6 pre-existing in partial A")
}

// TestRebuildMalformedInput verifies Rebuild returns an error (never panics)
// for structurally malformed input.
func TestRebuildMalformedInput(t *testing.T) {
	t.Run("unmarshalable tree bytes", func(t *testing.T) {
		// An overlong varint (more than 10 continuation-marked bytes):
		// the only malformed encoding pkg/model/og/util/varint.Uvarint
		// reports as an error rather than silently treating as truncated.
		overlongVarint := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}
		_, err := symbolref.Rebuild(overlongVarint, new(queryv1.SymbolRefTable), nil, 0)
		require.Error(t, err)
	})

	t.Run("mismatched pb parallel slices", func(t *testing.T) {
		table := symbolref.NewTable()
		loc := table.InternUnresolved("build1", "bin1", 0x1000)
		tree := new(model.LocationRefNameTree)
		tree.InsertStack(1, loc)
		rb := table.ResultBuilder()
		treeBytes := tree.Bytes(0, rb.KeepRef)

		badPB := &queryv1.SymbolRefTable{
			BuildIds:          []string{"build1"},
			BinaryNames:       []string{"bin1", "extra"},
			UnresolvedBuildId: []uint32{0},
			UnresolvedAddress: []uint64{0x1000},
		}
		_, err := symbolref.Rebuild(treeBytes, badPB, nil, 0)
		require.Error(t, err)
	})

	t.Run("out-of-range unresolved_build_id", func(t *testing.T) {
		table := symbolref.NewTable()
		loc := table.InternUnresolved("build1", "bin1", 0x1000)
		tree := new(model.LocationRefNameTree)
		tree.InsertStack(1, loc)
		rb := table.ResultBuilder()
		treeBytes := tree.Bytes(0, rb.KeepRef)

		badPB := &queryv1.SymbolRefTable{
			BuildIds:          []string{"build1"},
			BinaryNames:       []string{"bin1"},
			UnresolvedBuildId: []uint32{5},
			UnresolvedAddress: []uint64{0x1000},
		}
		_, err := symbolref.Rebuild(treeBytes, badPB, nil, 0)
		require.Error(t, err)
	})

	t.Run("tree ref beyond pb's range", func(t *testing.T) {
		tree := new(model.LocationRefNameTree)
		tree.InsertStack(1, model.LocationRefName(100))
		identity := func(ref model.LocationRefName) model.LocationRefName { return ref }
		treeBytes := tree.Bytes(0, identity)

		_, err := symbolref.Rebuild(treeBytes, new(queryv1.SymbolRefTable), nil, 0)
		require.Error(t, err)
	})
}
