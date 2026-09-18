package symdb

import (
	"fmt"
	"slices"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// Both benchmarks explicitly exercise the parent-pointer path. The existing
// in-memory Resolver benchmarks dispatch to the per-stack resolution path.
// Profile parsing, indexing and access to storage are outside timing.
func BenchmarkInsertStacktraces(b *testing.B) {
	fixture := newStacktraceReplayFixture(b, "testdata/big-profile.pb.gz")
	for _, maxNodes := range []int64{8 << 10, 16 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("MaxNodes%d", maxNodes), func(b *testing.B) {
			nodes := fixture.prepare(maxNodes)
			stacks, frames, total := stacktraceReplayStats(nodes, fixture.symbols)
			require.Positive(b, stacks)
			require.Positive(b, frames)
			require.Equal(b, fixture.total, total)
			var tree *model.StacktraceTree
			b.ReportAllocs()
			for b.Loop() {
				// Match buildTreeForStacktraceIDRange's initial capacity. In
				// particular, do not preallocate to the measured output size.
				tree = model.NewStacktraceTree(int(maxNodes))
				insertStacktraces(tree, nodes, fixture.symbols, addFunctionNames)
			}
			b.ReportMetric(float64(len(tree.Nodes)), "nodes/tree")
			b.ReportMetric(float64(cap(tree.Nodes))*float64(unsafe.Sizeof(model.StacktraceNode{})), "node-cap-B")
			reportStacktraceReplayMetrics(b, len(nodes), stacks, frames)
			var actualTotal int64
			for _, node := range tree.Nodes {
				actualTotal += node.Value
			}
			require.Equal(b, fixture.total, actualTotal)
		})
	}
}

func BenchmarkBuildTreeForStacktraceIDRange(b *testing.B) {
	fixture := newStacktraceReplayFixture(b, "testdata/big-profile.pb.gz")
	for _, maxNodes := range []int64{8 << 10, 16 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("MaxNodes%d", maxNodes), func(b *testing.B) {
			// Keep only the fingerprint, not the warm-up output tree, alive
			// while measuring. Each call obtains a new mutable Nodes() copy.
			expected := treeFingerprint(fixture.build(maxNodes))
			var tree *model.FunctionNameTree
			b.ReportAllocs()
			for b.Loop() {
				tree = fixture.build(maxNodes)
			}
			require.Equal(b, fixture.total, tree.Total())
			require.Equal(b, expected, treeFingerprint(tree))
			var outputNodes int
			tree.FormatNodeNames(func(name model.FunctionName) model.FunctionName {
				outputNodes++
				return name
			})
			b.ReportMetric(float64(outputNodes), "nodes/tree")
			b.ReportMetric(float64(len(fixture.stacktraces.IDs)), "samples/op")
			b.ReportMetric(float64(len(fixture.stacktraces.ParentPointerTree.(*parentPointerTree).nodes)), "source-nodes/op")
		})
	}
}

type stacktraceReplayFixture struct {
	stacktraces *StacktraceIDRange
	symbols     *Symbols
	total       int64
}

func newStacktraceReplayFixture(t testing.TB, path string) stacktraceReplayFixture {
	t.Helper()
	profile, err := pprof.OpenFile(path)
	require.NoError(t, err)
	profile.Normalize()
	// A standalone writer avoids starting a SymDB statistics goroutine and
	// avoids block IO. Convert to the same parent-pointer representation the
	// block reader supplies, then call the range builder directly.
	partition := NewPartitionWriter(0, DefaultConfig())
	indexed := partition.WriteProfileSymbols(profile.Profile)
	require.NotEmpty(t, indexed)
	// Queries aggregate by stack ID before building a range. The profile may
	// contain the same stack under different labels; SetNodeValues assigns,
	// rather than sums, so passing those samples directly would lose weight.
	appender := NewSampleAppender()
	appender.AppendMany(indexed[0].Samples.StacktraceIDs, indexed[0].Samples.Values)
	samples := appender.Samples()
	require.NotEmpty(t, samples.StacktraceIDs)
	source := partition.stacktraces.tree.nodes
	tree := newParentPointerTree(uint32(len(source)))
	for i, node := range source {
		tree.nodes[i] = pptNode{p: node.p, r: node.r}
	}
	symbols := partition.Symbols()
	// These benchmarks use the range directly. Do not retain the writer and
	// its duplicate insertion tree via the unused high-level resolver.
	symbols.Stacktraces = nil
	fixture := stacktraceReplayFixture{
		stacktraces: &StacktraceIDRange{
			IDs:               samples.StacktraceIDs,
			Samples:           samples,
			ParentPointerTree: tree,
		},
		symbols: symbols,
	}
	for _, value := range samples.Values {
		fixture.total += int64(value)
	}
	return fixture
}

func (f stacktraceReplayFixture) prepare(maxNodes int64) []Node {
	nodes := f.stacktraces.Nodes()
	f.stacktraces.SetNodeValues(nodes)
	propagateNodeValues(nodes)
	markNodesForTruncation(nodes, maxNodes*4)
	return nodes
}

func (f stacktraceReplayFixture) lookup(i int32) model.FunctionName {
	return model.FunctionName(f.symbols.Strings[i])
}

func (f stacktraceReplayFixture) build(maxNodes int64) *model.FunctionNameTree {
	return buildTreeForStacktraceIDRange[model.FunctionName, model.FunctionNameI](
		f.stacktraces, f.symbols, maxNodes, nil, f.lookup)
}

func stacktraceReplayStats(nodes []Node, symbols *Symbols) (stacks, frames int, total int64) {
	var stack []int32
	for i := 1; i < len(nodes); i++ {
		node := nodes[i]
		if node.Value > 0 && nodes[node.Parent].Location&truncationMark == 0 {
			stack = resolveStack(stack, nodes, int32(i), addFunctionNames, symbols)
			stacks++
			frames += len(stack)
			total += node.Value
		}
	}
	return stacks, frames, total
}

func reportStacktraceReplayMetrics(b *testing.B, nodes, stacks, frames int) {
	ns := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(ns/float64(stacks), "ns/stack")
	b.ReportMetric(ns/float64(frames), "ns/frame")
	b.ReportMetric(float64(stacks), "stacks/op")
	b.ReportMetric(float64(nodes), "source-nodes/op")
}

func TestStacktraceReplayFixture(t *testing.T) {
	// Keep the reference test small enough for normal unit-test runs. The
	// benchmarks also check totals on the large fixture after timing ends.
	fixture := newStacktraceReplayFixture(t, "testdata/profile.pb.gz")
	original := fixture.stacktraces.Nodes()
	for _, maxNodes := range []int64{8, 64, 8 << 10} {
		t.Run(fmt.Sprintf("MaxNodes%d", maxNodes), func(t *testing.T) {
			nodes := fixture.prepare(maxNodes)
			prepared := slices.Clone(nodes)
			stacks, frames, total := stacktraceReplayStats(nodes, fixture.symbols)
			require.Positive(t, stacks)
			require.Positive(t, frames)
			require.Equal(t, fixture.total, total)

			// Use the independent string-tree insertion implementation as a
			// reference for the intermediate tree, before final truncation.
			reference := new(model.FunctionNameTree)
			var locations []int32
			for i := 1; i < len(nodes); i++ {
				node := nodes[i]
				if node.Value <= 0 || nodes[node.Parent].Location&truncationMark != 0 {
					continue
				}
				locations = resolveStack(locations, nodes, int32(i), addFunctionNames, fixture.symbols)
				names := make([]model.FunctionName, len(locations))
				for j, location := range locations {
					name := model.OtherFunctionName
					if location != sentinel {
						name = fixture.lookup(location)
					}
					names[len(names)-1-j] = name
				}
				reference.InsertStack(node.Value, names...)
			}
			for pass := 0; pass < 2; pass++ {
				tree := model.NewStacktraceTree(int(maxNodes))
				insertStacktraces(tree, nodes, fixture.symbols, addFunctionNames)
				untruncated := model.TreeFromStacktraceTree[model.FunctionName, model.FunctionNameI](tree, 0, fixture.lookup)
				require.Equal(t, treeFingerprint(reference), treeFingerprint(untruncated))
				require.Equal(t, fixture.total, untruncated.Total())

				// Conversion may add truncation stubs; use a fresh tree.
				tree = model.NewStacktraceTree(int(maxNodes))
				insertStacktraces(tree, nodes, fixture.symbols, addFunctionNames)
				expected := model.TreeFromStacktraceTree[model.FunctionName, model.FunctionNameI](tree, maxNodes, fixture.lookup)
				actual := fixture.build(maxNodes)
				require.Equal(t, treeFingerprint(expected), treeFingerprint(actual))
				require.Equal(t, fixture.total, actual.Total())
				require.Equal(t, prepared, nodes, "replay must not mutate prepared input")
				require.Equal(t, original, fixture.stacktraces.Nodes(), "build must not mutate source")
			}
		})
	}
}
