package symbolref_test

// Jobs and Rebuild are not implemented in this package yet; tests exercising
// them, and the parts of the tests below that would need them, are deferred
// to a follow-up change.

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
)

// identity renders ref as a string uniquely identifying the resolved name or
// (buildID, binaryName, address) location it stands for, for stack-shape
// comparisons that must survive a change of ref space.
func identity(t *testing.T, pb *queryv1.SymbolRefTable, ref model.LocationRefName) string {
	t.Helper()
	if ref == model.OtherLocationRef {
		return "other"
	}
	names := pb.GetNames()
	if int(ref) < len(names) {
		return "name:" + names[ref]
	}
	i := int(ref) - len(names)
	require.Less(t, i, len(pb.GetUnresolvedAddress()), "ref %d out of range", ref)
	bi := pb.GetUnresolvedBuildId()[i]
	require.Less(t, int(bi), len(pb.GetBuildIds()), "build id index %d out of range", bi)
	return fmt.Sprintf("loc:%s|%s@%#x", pb.GetBuildIds()[bi], pb.GetBinaryNames()[bi], pb.GetUnresolvedAddress()[i])
}

// stacksByIdentity walks every stack in tree and sums self values keyed by
// the root-to-leaf sequence of identity() strings, so two trees using
// different (arbitrary) ref numbering can be compared for the same logical
// content.
func stacksByIdentity(t *testing.T, tree *model.LocationRefNameTree, pb *queryv1.SymbolRefTable) map[string]int64 {
	t.Helper()
	out := make(map[string]int64)
	tree.IterateStacks(func(_ model.LocationRefName, self int64, stack []model.LocationRefName) {
		slices.Reverse(stack)
		ids := make([]string, len(stack))
		for i, ref := range stack {
			ids[i] = identity(t, pb, ref)
		}
		out[strings.Join(ids, "/")] += self
	})
	return out
}

// marshalWire marshals tree via rb.KeepRef and unmarshals the result,
// returning both the marshaled bytes and a tree whose node names are in the
// wire encoding (matching whatever rb.Build subsequently writes), unlike
// tree's own (Table-internal) node names.
func marshalWire(t *testing.T, tree *model.LocationRefNameTree, rb *symbolref.ResultBuilder) ([]byte, *model.LocationRefNameTree) {
	t.Helper()
	b := tree.Bytes(0, rb.KeepRef)
	out, err := model.UnmarshalTree[model.LocationRefName, model.LocationRefNameI](b)
	require.NoError(t, err)
	return b, out
}

// testPartial is a marshaled (tree, table) pair, as a producer would send
// one over the wire in a queryv1.TreeReport.
type testPartial struct {
	tree []byte
	pb   *queryv1.SymbolRefTable
}

// buildPartial constructs a fresh Table, lets build populate a tree using
// the table's Intern* methods, and marshals both via ResultBuilder — the
// same sequence a real producer would follow.
func buildPartial(build func(*symbolref.Table) *model.LocationRefNameTree) testPartial {
	table := symbolref.NewTable()
	tree := build(table)
	rb := table.ResultBuilder()
	treeBytes := tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)
	return testPartial{tree: treeBytes, pb: pb}
}

// TestInternIdempotencyAndDedup verifies InternName and InternUnresolved are
// idempotent and content-addressed, and that the wire encoding they end up
// producing splits resolved and unresolved refs into disjoint ranges.
func TestInternIdempotencyAndDedup(t *testing.T) {
	table := symbolref.NewTable()

	foo1 := table.InternName("foo")
	bar := table.InternName("bar")
	loc2000a := table.InternUnresolved("build1", "bin1", 0x2000)
	foo2 := table.InternName("foo")
	loc1000a := table.InternUnresolved("build1", "bin1", 0x1000)
	foo3 := table.InternName("foo")
	loc2000b := table.InternUnresolved("build1", "bin1", 0x2000)
	loc1000b := table.InternUnresolved("build1", "bin1", 0x1000)
	loc1000c := table.InternUnresolved("build1", "bin1", 0x1000)
	loc1000renamed := table.InternUnresolved("build1", "bin1-renamed", 0x1000)

	require.Equal(t, foo1, foo2)
	require.Equal(t, foo1, foo3)
	require.Equal(t, loc1000a, loc1000b)
	require.Equal(t, loc1000a, loc1000c)
	require.Equal(t, loc2000a, loc2000b)
	require.NotEqual(t, foo1, bar)
	require.NotEqual(t, loc1000a, loc2000a)
	require.NotEqual(t, loc1000a, loc1000renamed,
		"same (buildID, addr) under a different binary name must be a distinct ref")

	tree := new(model.LocationRefNameTree)
	tree.InsertStack(1, foo1)
	tree.InsertStack(1, bar)
	tree.InsertStack(1, loc1000a)
	tree.InsertStack(1, loc2000a)

	// InternName/InternUnresolved's return values are Table's internal
	// representation, meaningful only as tree node names; the resolved/
	// unresolved range split applies to the wire encoding
	// ResultBuilder.KeepRef produces during a real marshal, so capture what
	// it actually assigns each ref.
	rb := table.ResultBuilder()
	wireOf := make(map[model.LocationRefName]model.LocationRefName)
	keepRef := func(ref model.LocationRefName) model.LocationRefName {
		out := rb.KeepRef(ref)
		wireOf[ref] = out
		return out
	}
	tree.Bytes(0, keepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	for _, resolved := range []model.LocationRefName{foo1, bar} {
		require.Less(t, int(wireOf[resolved]), len(pb.GetNames()), "resolved ref %d must be < len(pb.Names)", resolved)
	}
	for _, unresolved := range []model.LocationRefName{loc1000a, loc2000a} {
		require.GreaterOrEqual(t, int(wireOf[unresolved]), len(pb.GetNames()), "unresolved ref %d must be >= len(pb.Names)", unresolved)
	}
}

// TestAddRemapRoundTrip verifies Add's returned remap function, used with
// model.TreeMerger.MergeTreeBytes, preserves per-stack identity and values
// across a change of ref space.
func TestAddRemapRoundTrip(t *testing.T) {
	src := symbolref.NewTable()
	nameA := src.InternName("a")
	nameB := src.InternName("b")
	loc := src.InternUnresolved("build1", "bin1", 0x1000)

	srcTree := new(model.LocationRefNameTree)
	srcTree.InsertStack(3, nameA, nameB)
	srcTree.InsertStack(5, nameA, loc)
	srcTree.InsertStack(2, loc)

	srcRB := src.ResultBuilder()
	treeBytes, srcWireTree := marshalWire(t, srcTree, srcRB)
	srcPB := new(queryv1.SymbolRefTable)
	srcRB.Build(srcPB)
	want := stacksByIdentity(t, srcWireTree, srcPB)

	dst := symbolref.NewTable()
	remap, err := dst.Add(srcPB)
	require.NoError(t, err)

	merger := model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()
	require.NoError(t, merger.MergeTreeBytes(treeBytes, model.WithTreeMergeFormatNodeNames(remap)))

	dstRB := dst.ResultBuilder()
	_, dstWireTree := marshalWire(t, merger.Tree(), dstRB)
	dstPB := new(queryv1.SymbolRefTable)
	dstRB.Build(dstPB)

	got := stacksByIdentity(t, dstWireTree, dstPB)
	require.Equal(t, want, got)
}

// TestMergeDeterminismUnderPermutedArrival verifies that merging the same
// set of partials in any order produces the same per-stack values, the same
// set of resolved names, and the same unresolved wire encoding.
func TestMergeDeterminismUnderPermutedArrival(t *testing.T) {
	partials := []testPartial{
		buildPartial(func(table *symbolref.Table) *model.LocationRefNameTree {
			shared := table.InternName("shared_fn")
			p1 := table.InternName("p1_fn")
			loc := table.InternUnresolved("buildA", "binA", 0x100)
			tree := new(model.LocationRefNameTree)
			tree.InsertStack(3, shared, p1)
			tree.InsertStack(1, shared, loc)
			return tree
		}),
		buildPartial(func(table *symbolref.Table) *model.LocationRefNameTree {
			shared := table.InternName("shared_fn")
			p2 := table.InternName("p2_fn")
			locA := table.InternUnresolved("buildA", "binA", 0x100)
			locB := table.InternUnresolved("buildB", "binB", 0x200)
			tree := new(model.LocationRefNameTree)
			tree.InsertStack(4, shared, p2)
			tree.InsertStack(2, shared, locA)
			tree.InsertStack(6, p2, locB)
			return tree
		}),
		buildPartial(func(table *symbolref.Table) *model.LocationRefNameTree {
			p3 := table.InternName("p3_fn")
			locB := table.InternUnresolved("buildB", "binB", 0x200)
			// Same (buildID, addr) as partials 1 and 2, but stored under a
			// renamed binary: the kept name must not depend on which partial
			// arrives first.
			locA := table.InternUnresolved("buildA", "binA-renamed", 0x100)
			tree := new(model.LocationRefNameTree)
			tree.InsertStack(5, p3, locB)
			tree.InsertStack(7, p3, locA)
			return tree
		}),
	}

	orderings := [][3]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	}

	// merge reports the merged result's logical content: per-stack values,
	// the set of resolved names, and pb's content-sorted unresolved fields.
	// tree.Bytes' raw output and pb.Names' order are deliberately not
	// compared: both depend on the merged tree's sibling order, which
	// pkg/model's Tree sorts by raw ref value — and that raw value depends
	// on how many other names had already been interned in the destination
	// table, which is arrival-order-dependent even though the underlying
	// data is not.
	type mergeResult struct {
		stacks     map[string]int64
		names      map[string]struct{}
		unresolved *queryv1.SymbolRefTable
	}

	merge := func(ordering [3]int) mergeResult {
		dst := symbolref.NewTable()
		merger := model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()
		for _, i := range ordering {
			remap, err := dst.Add(partials[i].pb)
			require.NoError(t, err)
			require.NoError(t, merger.MergeTreeBytes(partials[i].tree, model.WithTreeMergeFormatNodeNames(remap)))
		}
		rb := dst.ResultBuilder()
		_, wire := marshalWire(t, merger.Tree(), rb)
		pb := new(queryv1.SymbolRefTable)
		rb.Build(pb)

		names := make(map[string]struct{}, len(pb.GetNames()))
		for _, n := range pb.GetNames() {
			names[n] = struct{}{}
		}
		return mergeResult{
			stacks: stacksByIdentity(t, wire, pb),
			names:  names,
			unresolved: &queryv1.SymbolRefTable{
				BuildIds:          pb.GetBuildIds(),
				BinaryNames:       pb.GetBinaryNames(),
				UnresolvedBuildId: pb.GetUnresolvedBuildId(),
				UnresolvedAddress: pb.GetUnresolvedAddress(),
			},
		}
	}

	want := merge(orderings[0])
	for _, ordering := range orderings[1:] {
		got := merge(ordering)
		require.Equal(t, want.stacks, got.stacks, "ordering %v produced different stack values", ordering)
		require.Equal(t, want.names, got.names, "ordering %v produced a different set of resolved names", ordering)
		require.True(t, proto.Equal(want.unresolved, got.unresolved),
			"ordering %v produced different unresolved wire ordering (the (buildID, binaryName, address) sort should make this arrival-independent): %v vs %v",
			ordering, want.unresolved, got.unresolved)
	}
}

// TestWireOrderingMultipleBuildIDs verifies unresolved wire entries are
// sorted by (buildID, binaryName, address) regardless of intern order, and
// that resolved names never contain a build ID string.
func TestWireOrderingMultipleBuildIDs(t *testing.T) {
	table := symbolref.NewTable()

	n1 := table.InternName("n1")
	locBbb2 := table.InternUnresolved("bbb", "bin-bbb", 0x2000)
	n2 := table.InternName("n2")
	locAaa1 := table.InternUnresolved("aaa", "bin-aaa", 0x1000)
	locCcc1 := table.InternUnresolved("ccc", "bin-ccc", 0x1000)
	locBbb1 := table.InternUnresolved("bbb", "bin-bbb", 0x1000)
	locAaa2 := table.InternUnresolved("aaa", "bin-aaa", 0x2000)
	// bbb again under a renamed binary, at an address between bbb's other
	// two: must sort after both bin-bbb entries, not between them.
	locBbbRenamed := table.InternUnresolved("bbb", "bin-bbb-renamed", 0x1500)

	tree := new(model.LocationRefNameTree)
	for _, ref := range []model.LocationRefName{n1, n2, locBbb2, locAaa1, locCcc1, locBbb1, locAaa2, locBbbRenamed} {
		tree.InsertStack(1, ref)
	}

	rb := table.ResultBuilder()
	tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	for _, name := range pb.GetNames() {
		require.NotContains(t, []string{"aaa", "bbb", "ccc"}, name, "pb.Names must never contain an unresolved build ID")
	}

	require.Equal(t, len(pb.GetUnresolvedBuildId()), len(pb.GetUnresolvedAddress()))
	keyAt := func(i int) (string, string, uint64) {
		row := pb.GetUnresolvedBuildId()[i]
		return pb.GetBuildIds()[row], pb.GetBinaryNames()[row], pb.GetUnresolvedAddress()[i]
	}
	for i := 1; i < len(pb.GetUnresolvedAddress()); i++ {
		prevBuildID, prevName, prevAddr := keyAt(i - 1)
		buildID, name, addr := keyAt(i)
		order := cmp.Or(
			cmp.Compare(prevBuildID, buildID),
			cmp.Compare(prevName, name),
			cmp.Compare(prevAddr, addr),
		)
		require.LessOrEqual(t, order, 0, "unresolved entries must be sorted by (buildID, binaryName, address); entry %d (%s, %s, %#x) sorts after (%s, %s, %#x)",
			i, prevBuildID, prevName, prevAddr, buildID, name, addr)
	}
}

// TestBinaryNamesRetainedAsStored verifies the renamed-binary case: the same
// build ID stored under two binary names keeps both rows, exactly as stored,
// rather than collapsing to whichever name arrived first.
func TestBinaryNamesRetainedAsStored(t *testing.T) {
	table := symbolref.NewTable()
	require.Zero(t, table.UnresolvedCount())
	a := table.InternUnresolved("build1", "pyroscope", 0x100)
	b := table.InternUnresolved("build1", "pyroscope-server", 0x100)
	require.NotEqual(t, a, b)
	require.Equal(t, 2, table.UnresolvedCount(),
		"the same location under two binary names is two distinct unresolved entries")

	tree := new(model.LocationRefNameTree)
	tree.InsertStack(1, a)
	tree.InsertStack(1, b)

	rb := table.ResultBuilder()
	tree.Bytes(0, rb.KeepRef)
	pb := rb.Build(nil)

	require.Equal(t, []string{"build1", "build1"}, pb.GetBuildIds())
	require.Equal(t, []string{"pyroscope", "pyroscope-server"}, pb.GetBinaryNames())
	require.Equal(t, []uint32{0, 1}, pb.GetUnresolvedBuildId())
	require.Equal(t, []uint64{0x100, 0x100}, pb.GetUnresolvedAddress())
}

// TestNegativeRefPassthrough verifies model.OtherLocationRef is never
// remapped, reassigned, or dropped by ResultBuilder.KeepRef or by an
// Add-returned remap function.
func TestNegativeRefPassthrough(t *testing.T) {
	table := symbolref.NewTable()
	table.InternName("foo")
	table.InternUnresolved("build1", "bin1", 0x1000)

	rb := table.ResultBuilder()
	require.Equal(t, model.OtherLocationRef, rb.KeepRef(model.OtherLocationRef))

	remap, err := table.Add(&queryv1.SymbolRefTable{
		Names:             []string{"bar"},
		BuildIds:          []string{"build2"},
		BinaryNames:       []string{"bin2"},
		UnresolvedBuildId: []uint32{0},
		UnresolvedAddress: []uint64{0x2000},
	})
	require.NoError(t, err)
	require.Equal(t, model.OtherLocationRef, remap(model.OtherLocationRef))
}

// TestEmptyTable verifies a fresh Table and its ResultBuilder behave safely
// with nothing interned. Jobs and Rebuild are not implemented in this
// package yet, so their behavior on an empty table is not covered here.
func TestEmptyTable(t *testing.T) {
	table := symbolref.NewTable()
	require.False(t, table.HasUnresolved())

	rb := table.ResultBuilder()
	built := rb.Build(nil)
	require.NotNil(t, built)
	require.Equal(t, []string{""}, built.GetNames())

	pb := new(queryv1.SymbolRefTable)
	symbolref.NewTable().ResultBuilder().Build(pb)
	// Build writes the full name snapshot, which always includes the
	// reserved ref-0 placeholder, so wire refs stay aligned with
	// len(pb.Names) no matter which refs a marshaled tree keeps.
	require.Equal(t, []string{""}, pb.GetNames())
	require.Empty(t, pb.GetBuildIds())
	require.Empty(t, pb.GetBinaryNames())
	require.Empty(t, pb.GetUnresolvedBuildId())
	require.Empty(t, pb.GetUnresolvedAddress())
}

// TestOnlyResolvedTable verifies a table built purely from InternName calls
// reports HasUnresolved() == false and produces no unresolved wire fields.
func TestOnlyResolvedTable(t *testing.T) {
	table := symbolref.NewTable()
	a := table.InternName("a")
	b := table.InternName("b")
	c := table.InternName("c")
	require.False(t, table.HasUnresolved())

	tree := new(model.LocationRefNameTree)
	tree.InsertStack(1, a)
	tree.InsertStack(1, b)
	tree.InsertStack(1, c)

	rb := table.ResultBuilder()
	// HasUnresolved() is false here, so real truncation (maxNodes > 0) is a
	// legitimate use of this same ResultBuilder.
	tree.Bytes(2, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	require.False(t, table.HasUnresolved())
	require.Empty(t, pb.GetBuildIds())
	require.Empty(t, pb.GetBinaryNames())
	require.Empty(t, pb.GetUnresolvedBuildId())
	require.Empty(t, pb.GetUnresolvedAddress())
}

// TestAddRemapIsTotal verifies the remap Add returns is safe for any ref,
// not only those pb describes: the merge machinery invokes it on a synthetic
// zero-valued name (model.Tree.FormatNodeNames visits its virtual root node)
// even when pb is nil or empty, and a skewed
// peer can send a tree whose refs exceed its table or carry negatives other
// than OtherLocationRef — which would alias the destination table's internal
// unresolved encoding (-2-idx) if passed through. All must degrade to the
// reserved ref 0, not panic or alias.
func TestAddRemapIsTotal(t *testing.T) {
	t.Run("nil and empty tables", func(t *testing.T) {
		for name, pb := range map[string]*queryv1.SymbolRefTable{
			"nil":   nil,
			"empty": new(queryv1.SymbolRefTable),
		} {
			t.Run(name, func(t *testing.T) {
				remap, err := symbolref.NewTable().Add(pb)
				require.NoError(t, err)
				require.Equal(t, model.LocationRefName(0), remap(0))
				require.Equal(t, model.LocationRefName(0), remap(7))
				require.Equal(t, model.OtherLocationRef, remap(model.OtherLocationRef))
				require.Equal(t, model.LocationRefName(0), remap(-2))
			})
		}
	})

	t.Run("ref beyond the table degrades", func(t *testing.T) {
		p := buildPartial(func(table *symbolref.Table) *model.LocationRefNameTree {
			n := table.InternName("fn")
			tree := new(model.LocationRefNameTree)
			tree.InsertStack(1, n)
			return tree
		})
		remap, err := symbolref.NewTable().Add(p.pb)
		require.NoError(t, err)
		outOfRange := model.LocationRefName(int32(len(p.pb.GetNames())) + int32(len(p.pb.GetUnresolvedAddress())))
		require.Equal(t, model.LocationRefName(0), remap(outOfRange))
	})

	t.Run("negative wire ref other than OtherLocationRef degrades", func(t *testing.T) {
		// The destination has an unresolved entry at internal ref -2; a wire
		// ref -2 from a malformed tree must not alias it.
		dst := symbolref.NewTable()
		dst.InternUnresolved("buildA", "binA", 0x100)
		remap, err := dst.Add(new(queryv1.SymbolRefTable))
		require.NoError(t, err)
		require.Equal(t, model.LocationRefName(0), remap(-2))
	})

	t.Run("merging tree bytes against an absent table does not panic", func(t *testing.T) {
		p := buildPartial(func(table *symbolref.Table) *model.LocationRefNameTree {
			n := table.InternName("fn")
			tree := new(model.LocationRefNameTree)
			tree.InsertStack(1, n)
			return tree
		})
		remap, err := symbolref.NewTable().Add(nil)
		require.NoError(t, err)
		merger := model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()
		require.NoError(t, merger.MergeTreeBytes(p.tree, model.WithTreeMergeFormatNodeNames(remap)))
	})
}

// TestPlainFunctionNameAbsorption verifies a plain FunctionName-tree partial
// (as an old backend, or a fully-symbolized dataset, would produce) can be
// absorbed into the symbol-ref space via InternName and merged alongside a
// genuine unresolved partial without sample loss. The Rebuild-dependent
// assertion (that the absorbed samples also survive a final rebuild) is
// deferred: Rebuild is not implemented in this package yet.
func TestPlainFunctionNameAbsorption(t *testing.T) {
	plainTree := new(model.FunctionNameTree)
	plainTree.InsertStack(7, model.FunctionName("main"), model.FunctionName("plainFn"))
	plainTreeBytes := plainTree.Bytes(0, nil)

	refTable := symbolref.NewTable()
	locName := refTable.InternName("main")
	locUnresolved := refTable.InternUnresolved("buildB", "binB", 0x9000)
	refTree := new(model.LocationRefNameTree)
	refTree.InsertStack(4, locName, locUnresolved)
	refRB := refTable.ResultBuilder()
	refTreeBytes := refTree.Bytes(0, refRB.KeepRef)
	refPB := new(queryv1.SymbolRefTable)
	refRB.Build(refPB)

	dst := symbolref.NewTable()
	merger := model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()

	// Unmarshal the plain partial and re-insert every stack into a
	// LocationRefNameTree via InternName.
	plain, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](plainTreeBytes)
	require.NoError(t, err)
	absorbed := new(model.LocationRefNameTree)
	plain.IterateStacks(func(_ model.FunctionName, self int64, stack []model.FunctionName) {
		slices.Reverse(stack)
		refs := make([]model.LocationRefName, len(stack))
		for i, n := range stack {
			refs[i] = dst.InternName(string(n))
		}
		absorbed.InsertStack(self, refs...)
	})
	merger.MergeTree(absorbed)

	remap, err := dst.Add(refPB)
	require.NoError(t, err)
	require.NoError(t, merger.MergeTreeBytes(refTreeBytes, model.WithTreeMergeFormatNodeNames(remap)))

	require.True(t, dst.HasUnresolved())

	rb := dst.ResultBuilder()
	_, mergedWireTree := marshalWire(t, merger.Tree(), rb)
	mergedPB := new(queryv1.SymbolRefTable)
	rb.Build(mergedPB)

	stacks := stacksByIdentity(t, mergedWireTree, mergedPB)
	require.Equal(t, int64(7), stacks["name:main/name:plainFn"], "partial A's stack must survive absorption with its original self value")
}

// Wire refs must decode to the right SymbolRefTable rows regardless of
// which refs the marshaled tree keeps: a truncating marshal (or any keep
// set smaller than the table) must not shift unresolved refs relative to
// pb.Names, since the wire contract reads unresolved entry i as
// ref - len(names).
func TestResultBuilder_wireRefsIndependentOfKeptSet(t *testing.T) {
	table := symbolref.NewTable()
	dropped := table.InternName("dropped_by_truncation")
	kept := table.InternName("kept")
	droppedU := table.InternUnresolved("build-id-a", "liba.so", 0x10)
	keptU := table.InternUnresolved("build-id-b", "libb.so", 0x20)

	rb := table.ResultBuilder()
	keptWire := rb.KeepRef(kept)
	keptUWire := rb.KeepRef(keptU)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	require.Less(t, int(keptWire), len(pb.Names))
	require.Equal(t, "kept", pb.Names[keptWire])

	i := int(keptUWire) - len(pb.Names)
	require.GreaterOrEqual(t, i, 0, "unresolved wire ref must be >= len(pb.Names)")
	require.Less(t, i, len(pb.UnresolvedAddress))
	require.Equal(t, uint64(0x20), pb.UnresolvedAddress[i])
	require.Equal(t, "build-id-b", pb.BuildIds[pb.UnresolvedBuildId[i]])

	_, _ = dropped, droppedU // interned but never kept: must not shift the wire encoding
}
