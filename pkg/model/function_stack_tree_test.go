package model

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type testStack struct {
	names []string
	value int64
}

// stackTreeFromPartitions mimics symdb: each partition has its own name table
// and inserts leaf-first stacks of name IDs into a StacktraceTree.
func stackTreeFromPartitions(partitions [][]testStack) *FunctionStackTree {
	t := NewFunctionStackTree()
	for _, stacks := range partitions {
		names := []string{""}
		ids := make(map[string]int32)
		st := NewStacktraceTree(16)
		for _, s := range stacks {
			leafFirst := make([]int32, 0, len(s.names))
			for _, name := range slices.Backward(s.names) {
				id, ok := ids[name]
				if !ok {
					id = int32(len(names))
					ids[name] = id
					names = append(names, name)
				}
				leafFirst = append(leafFirst, id)
			}
			st.Insert(leafFirst, s.value)
		}
		t.MergeStacktraceTree(st, names)
	}
	return t
}

func referenceFunctionTree(partitions [][]testStack, path []string, o *typesv1.FunctionTreeOptions) *typesv1.FunctionTree {
	reports := NewFunctionTreeMerger(nil)
	merger := NewFunctionTreeMerger(o)
	for _, stacks := range partitions {
		b := NewFunctionTreeBuilder(path, o, func(s string) string { return s })
		for _, s := range stacks {
			if o.GetSelection() == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH &&
				(len(s.names) < len(path) || !slices.Equal(s.names[:len(path)], path)) {
				continue
			}
			b.Insert(s.names, s.value)
		}
		merger.Merge(b.Tree())
	}
	reports.Merge(merger.Tree())
	return reports.Tree()
}

func referenceFunctions(partitions [][]testStack, path []string) *typesv1.FunctionTable {
	merger := NewFunctionTableMerger()
	for _, stacks := range partitions {
		table := new(typesv1.FunctionTable)
		stats := make(map[string]*typesv1.FunctionStats)
		for _, s := range stacks {
			if s.value <= 0 || len(s.names) < len(path) || !slices.Equal(s.names[:len(path)], path) {
				continue
			}
			table.Total += s.value
			seen := make(map[string]bool)
			for i, name := range s.names {
				f := stats[name]
				if f == nil {
					f = &typesv1.FunctionStats{Name: name}
					stats[name] = f
				}
				if !seen[name] {
					f.Total += s.value
					seen[name] = true
				}
				if i == len(s.names)-1 {
					f.Self += s.value
				}
			}
		}
		for _, f := range stats {
			table.Functions = append(table.Functions, f)
		}
		merger.Merge(table)
	}
	return reportFunctionTable(merger)
}

func reportFunctionTable(m *FunctionTableMerger) *typesv1.FunctionTable {
	reports := NewFunctionTableMerger()
	reports.Merge(m.Table())
	return reports.Table()
}

func randomPartitions(rng *rand.Rand) [][]testStack {
	alphabet := []string{"main", "a", "b", "c", "other", ""}
	partitions := make([][]testStack, 1+rng.Intn(3))
	for p := range partitions {
		for range 1 + rng.Intn(40) {
			names := make([]string, rng.Intn(7))
			for i := range names {
				names[i] = alphabet[rng.Intn(len(alphabet))]
			}
			partitions[p] = append(partitions[p], testStack{names: names, value: 1 + rng.Int63n(100)})
		}
	}
	return partitions
}

func testSelections(partitions [][]testStack) [][]string {
	seen := map[string]bool{}
	selections := [][]string{nil, {"missing"}, {"main", "missing"}}
	for _, stacks := range partitions {
		for _, s := range stacks {
			for i := range s.names {
				for j := i + 1; j <= len(s.names) && j-i <= 3; j++ {
					k := fmt.Sprintf("%q", s.names[i:j])
					if !seen[k] {
						seen[k] = true
						selections = append(selections, slices.Clone(s.names[i:j]))
					}
				}
			}
		}
	}
	return selections
}

func roundTrip(t *testing.T, tree *FunctionStackTree) *FunctionStackIndex {
	t.Helper()
	b, err := proto.Marshal(tree.Proto())
	require.NoError(t, err)
	var decoded queryv1.FunctionStackTree
	require.NoError(t, proto.Unmarshal(b, &decoded))
	merged := NewFunctionStackTree()
	require.NoError(t, merged.MergeProto(&decoded))
	x, err := NewFunctionStackIndex(merged.Proto())
	require.NoError(t, err)
	return x
}

func TestFunctionStackIndexMatchesProjections(t *testing.T) {
	directions := []typesv1.FunctionTreeDirection{
		typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS,
		typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH,
	}
	depths := []*int32{nil, proto.Int32(0), proto.Int32(1), proto.Int32(2), proto.Int32(5)}
	for seed := int64(0); seed < 60; seed++ {
		rng := rand.New(rand.NewSource(seed))
		partitions := randomPartitions(rng)
		x := roundTrip(t, stackTreeFromPartitions(partitions))
		for _, path := range testSelections(partitions) {
			require.True(t, referenceFunctions(partitions, path).EqualVT(x.Functions(path)), "seed %d functions %q", seed, path)
			for _, direction := range directions {
				for _, selection := range []typesv1.FunctionTreeSelection{
					typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
					typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
				} {
					if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH && direction != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES {
						continue
					}
					if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN &&
						(len(path) == 0 || (direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH && len(path) != 1)) {
						continue
					}
					for _, depth := range depths {
						o := &typesv1.FunctionTreeOptions{Direction: direction, Selection: selection, MaxDepth: depth}
						want := referenceFunctionTree(partitions, path, o)
						got := x.FunctionTree(path, o)
						require.True(t, want.EqualVT(got), "seed %d %s %s depth %v path %q\nwant %v\ngot  %v", seed, selection, direction, depth, path, want, got)
					}
				}
			}
		}
	}
}

func TestFunctionStackTreeEmptyStacks(t *testing.T) {
	partitions := [][]testStack{{{names: nil, value: 3}, {names: []string{"main"}, value: 4}}}
	x := roundTrip(t, stackTreeFromPartitions(partitions))
	table := x.Functions(nil)
	require.Equal(t, int64(7), table.Total)
	require.Equal(t, int64(1), table.TotalFunctions)
	o := &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
	}
	root := x.FunctionTree(nil, o).Callees.Root
	require.Equal(t, int64(7), root.Total)
	require.Equal(t, int64(3), root.Self)
	require.True(t, referenceFunctionTree(partitions, nil, o).EqualVT(x.FunctionTree(nil, o)))
}

func TestFunctionStackTreeMergeAndEncode(t *testing.T) {
	a := stackTreeFromPartitions([][]testStack{{{names: []string{"main", "a"}, value: 1}, {names: []string{"main"}, value: 2}}})
	b := stackTreeFromPartitions([][]testStack{{{names: []string{"main", "a"}, value: 3}, {names: []string{"x"}, value: 4}}})
	m := NewFunctionStackTree()
	require.NoError(t, m.MergeProto(a.Proto()))
	require.NoError(t, m.MergeProto(b.Proto()))
	require.Equal(t, 4, m.Len())
	x, err := NewFunctionStackIndex(m.Proto())
	require.NoError(t, err)
	require.Equal(t, int64(10), x.Functions(nil).Total)
	require.Equal(t, int64(4), x.Functions([]string{"main", "a"}).Total)
}

func TestFunctionStackTreeMalformed(t *testing.T) {
	valid := stackTreeFromPartitions([][]testStack{{{names: []string{"main", "a"}, value: 1}}}).Proto()
	for name, mutate := range map[string]func(*queryv1.FunctionStackTree){
		"empty":              func(x *queryv1.FunctionStackTree) { *x = queryv1.FunctionStackTree{} },
		"length mismatch":    func(x *queryv1.FunctionStackTree) { x.NodeSelf = x.NodeSelf[:1] },
		"root name":          func(x *queryv1.FunctionStackTree) { x.NodeName[0] = 1 },
		"reserved root name": func(x *queryv1.FunctionStackTree) { x.Names[0] = "main" },
		"name out of range":  func(x *queryv1.FunctionStackTree) { x.NodeName[1] = 100 },
		"root reference":     func(x *queryv1.FunctionStackTree) { x.NodeName[1] = 0 },
		"too many children":  func(x *queryv1.FunctionStackTree) { x.NodeChildren[2] = 1 },
		"too few children":   func(x *queryv1.FunctionStackTree) { x.NodeChildren[0] = 0 },
		"duplicate name":     func(x *queryv1.FunctionStackTree) { x.Names = append(x.Names, x.Names[1]) },
		"value overflow":     func(x *queryv1.FunctionStackTree) { x.NodeSelf[2] = 1 << 63 },
	} {
		t.Run(name, func(t *testing.T) {
			x := valid.CloneVT()
			mutate(x)
			_, err := NewFunctionStackIndex(x)
			require.ErrorIs(t, err, ErrMalformedFunctionStackTree)
			if name != "duplicate name" && name != "value overflow" {
				require.ErrorIs(t, NewFunctionStackTree().MergeProto(x), ErrMalformedFunctionStackTree)
			}
		})
	}
}
