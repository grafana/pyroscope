package querybackend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
)

// TestDatasetUnsymbolized covers datasetUnsymbolized's label-pair scanning,
// including a compacted dataset that carries more than one label set.
func TestDatasetUnsymbolized(t *testing.T) {
	// index: 0="" 1="__unsymbolized__" 2="true" 3="other_label" 4="other_value"
	md := &metastorev1.BlockMeta{
		StringTable: []string{"", "__unsymbolized__", "true", "other_label", "other_value"},
	}
	tests := []struct {
		name   string
		labels []int32
		want   bool
	}{
		{name: "no labels", labels: nil, want: false},
		{name: "unrelated label only", labels: []int32{1, 3, 4}, want: false},
		{name: "unsymbolized=true", labels: []int32{1, 1, 2}, want: true},
		{
			name:   "unsymbolized=true among other label sets (compacted dataset)",
			labels: []int32{1, 3, 4, 1, 1, 2},
			want:   true,
		},
		{
			name:   "unsymbolized label name present but value is not true",
			labels: []int32{1, 1, 3},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &metastorev1.Dataset{Labels: tt.labels}
			assert.Equal(t, tt.want, datasetUnsymbolized(md, ds))
		})
	}
}

// plainReport builds a symbol_refs-mode partial report that took the plain
// FunctionName path (native dataset, or a degenerate labeled one): no
// SymbolRefs, Tree bytes are a marshaled FunctionNameTree.
func plainReport(maxNodes int64, stacks map[string]int64) *queryv1.Report {
	tree := new(model.FunctionNameTree)
	for stack, self := range stacks {
		tree.InsertStack(self, model.FunctionName(stack))
	}
	return &queryv1.Report{
		Tree: &queryv1.TreeReport{
			Query: &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_REFS, MaxNodes: maxNodes},
			Tree:  tree.Bytes(0, nil),
		},
	}
}

// refReport builds a symbol_refs-mode partial report with a genuine
// unresolved entry: a single-frame stack referencing (buildID, addr).
func refReport(maxNodes int64, self int64, buildID, binaryName string, addr uint64) *queryv1.Report {
	table := symbolref.NewTable()
	ref := table.InternUnresolved(buildID, binaryName, addr)
	tree := new(model.LocationRefNameTree)
	tree.InsertStack(self, ref)
	rb := table.ResultBuilder()
	treeBytes := tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)
	return &queryv1.Report{
		Tree: &queryv1.TreeReport{
			Query:      &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_REFS, MaxNodes: maxNodes},
			Tree:       treeBytes,
			SymbolRefs: pb,
		},
	}
}

// aggregate runs every report through a fresh treeAggregator and returns
// the built TreeReport.
func aggregate(t *testing.T, reports ...*queryv1.Report) *queryv1.TreeReport {
	t.Helper()
	a := newTreeAggregator(&queryv1.InvokeRequest{}).(*treeAggregator)
	for _, r := range reports {
		require.NoError(t, a.aggregate(r))
	}
	return a.build().Tree
}

// buildSymbolRefs aggregates reports and unmarshals the result as a
// LocationRefNameTree, asserting a SymbolRefTable is attached (i.e. the
// merge still has something unresolved; see TestTreeAggregator_SymbolRefs_
// AllPlain and _OtherPreservation for the opposite, collapsed-to-plain
// case). Every tree here has gone through a real marshal (build always
// calls Tree.Bytes), which always keeps ref 0 as a side effect of
// marshaling the format's virtual root node: pb.Names[0] is therefore
// always "", the reserved sentinel (see symbolref.NewTable's doc comment),
// on top of whatever real names the test cares about.
func buildSymbolRefs(t *testing.T, reports ...*queryv1.Report) (*queryv1.SymbolRefTable, *model.LocationRefNameTree) {
	t.Helper()
	report := aggregate(t, reports...)
	require.NotNil(t, report.SymbolRefs)
	require.NotEmpty(t, report.SymbolRefs.Names)
	assert.Equal(t, "", report.SymbolRefs.Names[0], "index 0 is always the reserved sentinel")
	tree, err := model.UnmarshalTree[model.LocationRefName, model.LocationRefNameI](report.Tree)
	require.NoError(t, err)
	return report.SymbolRefs, tree
}

// realNames returns pb.Names with the reserved index-0 sentinel dropped.
func realNames(pb *queryv1.SymbolRefTable) []string {
	return pb.Names[1:]
}

// namesByRef resolves a leaf's own ref (the name IterateStacks passes to
// its callback) to its SymbolRefTable name, using placeholders for the
// truncation sentinel and unresolved entries.
func namesByRef(pb *queryv1.SymbolRefTable, ref model.LocationRefName) string {
	switch {
	case ref == model.OtherLocationRef:
		return "<other>"
	case ref >= 0 && int(ref) < len(pb.Names):
		return pb.Names[ref]
	default:
		return "<unresolved>"
	}
}

// TestTreeAggregator_SymbolRefs_AllPlain covers every partial taking the
// plain FunctionName path: the merged table has nothing unresolved, so
// build() collapses back to a plain, truncatable FunctionName report
// instead of forcing the SymbolRefTable format on it.
func TestTreeAggregator_SymbolRefs_AllPlain(t *testing.T) {
	report := aggregate(t, plainReport(2, map[string]int64{
		"f1": 50, "f2": 40, "f3": 30, "f4": 20, "f5": 10,
	}))
	require.Nil(t, report.SymbolRefs, "nothing unresolved: must not force the SymbolRefTable format")

	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](report.Tree)
	require.NoError(t, err)

	var otherTotal, total int64
	var distinctNames int
	tree.IterateStacks(func(name model.FunctionName, self int64, _ []model.FunctionName) {
		total += self
		if name == model.OtherFunctionName {
			otherTotal += self
		} else {
			distinctNames++
		}
	})
	assert.Equal(t, int64(150), total, "total sample value must be preserved across truncation")
	assert.LessOrEqual(t, distinctNames, 2, "maxNodes=2 must truncate for real once nothing is unresolved")
	assert.Greater(t, otherTotal, int64(0), "the truncated nodes must be folded into the other bucket")
}

// TestTreeAggregator_SymbolRefs_AllRef covers every partial carrying a
// genuine unresolved entry: build() must not truncate, regardless of
// MaxNodes, since doing so would be unsound before resolution.
func TestTreeAggregator_SymbolRefs_AllRef(t *testing.T) {
	pb, tree := buildSymbolRefs(t,
		refReport(2, 50, "buildA", "binA", 0x1000),
		refReport(2, 40, "buildB", "binB", 0x2000),
		refReport(2, 30, "buildC", "binC", 0x3000),
	)

	require.Len(t, pb.UnresolvedBuildId, 3, "maxNodes must not drop unresolved entries")
	assert.Equal(t, int64(120), tree.Total())

	var sawOther bool
	tree.IterateStacks(func(name model.LocationRefName, _ int64, _ []model.LocationRefName) {
		if name == model.OtherLocationRef {
			sawOther = true
		}
	})
	assert.False(t, sawOther, "a tree with unresolved entries must not be truncated")
}

// TestTreeAggregator_SymbolRefs_Mixed covers a query that spans datasets in
// different states (rollout skew): some partials plain (native or
// degenerate), some carrying genuine unresolved entries. Every partial's
// value must be preserved in the merged tree, and truncation must stay
// disabled because at least one unresolved entry survives.
func TestTreeAggregator_SymbolRefs_Mixed(t *testing.T) {
	pb, tree := buildSymbolRefs(t,
		plainReport(1, map[string]int64{"main": 7}),
		refReport(1, 4, "buildB", "binB", 0x9000),
	)

	require.Len(t, pb.UnresolvedBuildId, 1)
	assert.Equal(t, []string{"main"}, realNames(pb))
	assert.Equal(t, int64(11), tree.Total())

	totals := make(map[string]int64)
	tree.IterateStacks(func(name model.LocationRefName, self int64, _ []model.LocationRefName) {
		totals[namesByRef(pb, name)] += self
	})
	assert.Equal(t, int64(7), totals["main"], "the plain partial's stack must survive absorption with its original value")
	assert.Equal(t, int64(4), totals["<unresolved>"], "the ref partial's stack must survive the merge with its original value")
}

// TestTreeAggregator_SymbolRefs_OtherPreservation covers a plain partial
// that already carries the truncation sentinel (model.OtherFunctionName,
// "other"): absorption must map it to model.OtherLocationRef, and, since
// nothing else in this merge is unresolved, build()'s conversion back to
// FunctionName format must map OtherLocationRef back to OtherFunctionName,
// never through an ordinary resolvable name.
func TestTreeAggregator_SymbolRefs_OtherPreservation(t *testing.T) {
	report := aggregate(t, plainReport(0, map[string]int64{
		"main":                          5,
		string(model.OtherFunctionName): 3,
	}))
	require.Nil(t, report.SymbolRefs)

	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](report.Tree)
	require.NoError(t, err)

	var otherSelf int64
	tree.IterateStacks(func(name model.FunctionName, self int64, _ []model.FunctionName) {
		if name == model.OtherFunctionName {
			otherSelf = self
		}
	})
	assert.Equal(t, int64(3), otherSelf, "the sentinel's value must round-trip through OtherLocationRef and back")
}
