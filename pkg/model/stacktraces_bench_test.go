package model

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// One operation builds or replays an entire batch, not one stack. Inputs and
// access orders are generated outside timing. See stacktraces_bench.md.
func BenchmarkStacktraceTreeInsert(b *testing.B) {
	for _, workload := range stacktraceInsertWorkloads() {
		b.Run(workload.name, func(b *testing.B) {
			order := rand.New(rand.NewSource(0)).Perm(len(workload.stacks))
			build := stacktraceInsertBatch(workload.stacks, order)
			nodes := len(stacktraceInsertReference(build))
			for _, capacity := range []struct {
				name string
				size int
			}{{"SmallCapacity", 1}, {"ExactCapacity", nodes}} {
				b.Run("BuildFresh/"+capacity.name, func(b *testing.B) {
					var tree *StacktraceTree
					b.ReportAllocs()
					for b.Loop() {
						tree = NewStacktraceTree(capacity.size)
						insertStacktraceBatch(tree, build)
					}
					require.Len(b, tree.Nodes, nodes)
					reportStacktraceInsertMetrics(b, tree, build)
				})
			}

			// After warming, the initial capacity is immaterial. Include Reset
			// in timing so future auxiliary indexes must pay their cleanup cost.
			b.Run("ResetBuild", func(b *testing.B) {
				tree := NewStacktraceTree(1)
				insertStacktraceBatch(tree, build)
				b.ReportAllocs()
				for b.Loop() {
					tree.Reset()
					insertStacktraceBatch(tree, build)
				}
				require.Len(b, tree.Nodes, nodes)
				reportStacktraceInsertMetrics(b, tree, build)
			})

			accesses := []string{"Uniform"}
			if workload.wide {
				accesses = append(accesses, "Skewed")
			}
			for _, access := range accesses {
				b.Run("LookupExisting/"+access, func(b *testing.B) {
					tree := NewStacktraceTree(nodes)
					insertStacktraceBatch(tree, build)
					replay := stacktraceReplayBatch(build, access == "Skewed")
					b.ReportAllocs()
					for b.Loop() {
						insertStacktraceBatch(tree, replay)
					}
					require.Len(b, tree.Nodes, nodes)
					reportStacktraceInsertMetrics(b, tree, replay)
				})
			}
		})
	}
}

type stacktraceInsertWorkload struct {
	name   string
	stacks [][]int32 // Leaf first, as required by Insert.
	wide   bool
}

func stacktraceInsertWorkloads() []stacktraceInsertWorkload {
	var workloads []stacktraceInsertWorkload
	for _, depth := range []int{16, 64, 256} {
		workloads = append(workloads, stacktraceInsertWorkload{
			name: fmt.Sprintf("Chain/Depth%d", depth), stacks: stacktraceBalancedStacks(1, depth),
		})
	}
	for _, prefix := range []int{0, 32} {
		for _, fanout := range []int{8, 64, 512, 4096} {
			stacks := make([][]int32, fanout)
			for i := range stacks {
				stack := make([]int32, prefix+1)
				stack[0] = int32(i)
				for j := 1; j <= prefix; j++ {
					stack[j] = int32(fanout + j)
				}
				stacks[i] = stack
			}
			workloads = append(workloads, stacktraceInsertWorkload{
				name: fmt.Sprintf("Wide/Prefix%d/Fanout%d", prefix, fanout), stacks: stacks, wide: true,
			})
		}
	}
	for _, shape := range []struct{ branching, depth int }{{2, 12}, {8, 4}} {
		workloads = append(workloads, stacktraceInsertWorkload{
			name:   fmt.Sprintf("Balanced/Branching%d/Depth%d", shape.branching, shape.depth),
			stacks: stacktraceBalancedStacks(shape.branching, shape.depth),
		})
	}
	// Many narrow branches, a few wide parents, variable depths, and the same
	// location IDs under different parents. Each leaf path is distinct.
	var mixed [][]int32
	for parent := 0; parent < 64; parent++ {
		fanout := 2
		if parent%16 == 0 {
			fanout = 256
		}
		for child := 0; child < fanout; child++ {
			stack := []int32{int32(parent), int32(child)}
			for depth := 0; depth < 4+parent%32; depth++ {
				stack = append(stack, int32(depth%4))
			}
			slices.Reverse(stack)
			mixed = append(mixed, stack)
		}
	}
	return append(workloads, stacktraceInsertWorkload{name: "Mixed", stacks: mixed})
}

func stacktraceBalancedStacks(branching, depth int) [][]int32 {
	var stacks [][]int32
	var visit func([]int32)
	visit = func(path []int32) {
		if len(path) == depth {
			stack := slices.Clone(path)
			slices.Reverse(stack)
			stacks = append(stacks, stack)
			return
		}
		for child := 0; child < branching; child++ {
			// Deliberately reuse IDs under different parents and at different
			// depths; a location-only index would incorrectly merge nodes.
			visit(append(path, int32(child)))
		}
	}
	visit(make([]int32, 0, depth))
	return stacks
}

func stacktraceInsertBatch(stacks [][]int32, order []int) [][]int32 {
	batch := make([][]int32, len(order))
	for i, index := range order {
		batch[i] = stacks[index]
	}
	return batch
}

func stacktraceReplayBatch(build [][]int32, skewed bool) [][]int32 {
	// At least 256 lookups amortize loop overhead for single-chain workloads.
	order := rand.New(rand.NewSource(1)).Perm(len(build))
	batch := make([][]int32, max(256, len(build)))
	for i := range batch {
		index := order[i%len(order)]
		if skewed && i%5 != 0 {
			// 80% of accesses hit the first two or last two inserted children.
			// Including late siblings avoids an unrealistically cheap hot set.
			hot := [...]int{0, 1, len(build) - 2, len(build) - 1}
			index = hot[i%len(hot)]
		}
		batch[i] = build[index]
	}
	return batch
}

func insertStacktraceBatch(tree *StacktraceTree, batch [][]int32) {
	for _, stack := range batch {
		tree.Insert(stack, 1)
	}
}

func reportStacktraceInsertMetrics(b *testing.B, tree *StacktraceTree, batch [][]int32) {
	var frames int
	for _, stack := range batch {
		frames += len(stack)
	}
	ns := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(ns/float64(len(batch)), "ns/stack")
	b.ReportMetric(ns/float64(frames), "ns/frame")
	b.ReportMetric(float64(len(batch)), "stacks/op")
	b.ReportMetric(float64(len(tree.Nodes)), "nodes/tree")
	// Backing storage only, excluding allocator rounding and tree headers.
	nodeBytes := cap(tree.Nodes) * int(unsafe.Sizeof(StacktraceNode{}))
	edgeBytes := cap(tree.edges.slots) * int(unsafe.Sizeof(uint32(0)))
	b.ReportMetric(float64(nodeBytes), "node-cap-B")
	b.ReportMetric(float64(edgeBytes), "edge-cap-B")
	b.ReportMetric(float64(nodeBytes+edgeBytes), "retained-cap-B")
}

// This reference uses a map per node, independent of the flat slice and sibling
// links being benchmarked. It also defines the exact preallocation size.
type stacktraceReferenceNode struct {
	children map[int32]int
	self     int64
	total    int64
}

func stacktraceInsertReference(batch [][]int32) []stacktraceReferenceNode {
	nodes := []stacktraceReferenceNode{{children: make(map[int32]int)}}
	for _, stack := range batch {
		parent := 0
		for j := len(stack) - 1; j >= 0; j-- {
			child, ok := nodes[parent].children[stack[j]]
			if !ok {
				child = len(nodes)
				nodes[parent].children[stack[j]] = child
				nodes = append(nodes, stacktraceReferenceNode{children: make(map[int32]int)})
			}
			nodes[child].total++
			parent = child
		}
		nodes[parent].self++
	}
	return nodes
}

func requireStacktraceInsertReference(t testing.TB, tree *StacktraceTree, batch [][]int32) {
	t.Helper()
	want := stacktraceInsertReference(batch)
	require.Len(t, tree.Nodes, len(want))
	type pair struct{ actual, expected int }
	pending := []pair{{}}
	seen := make(map[int]bool, len(want))
	for len(pending) > 0 {
		p := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		require.False(t, seen[p.actual], "cycle or multiply linked node")
		seen[p.actual] = true
		actual, expected := tree.Nodes[p.actual], want[p.expected]
		require.Equal(t, expected.self, actual.Value)
		require.Equal(t, expected.total, actual.Total)
		children := make(map[int32]bool)
		for i := actual.FirstChild; i != sentinel; i = tree.Nodes[i].NextSibling {
			require.Greater(t, i, int32(0))
			require.Less(t, int(i), len(tree.Nodes))
			child := tree.Nodes[i]
			require.Equal(t, int32(p.actual), child.Parent)
			require.False(t, children[child.Location], "duplicate child or sibling cycle")
			children[child.Location] = true
			expectedChild, ok := expected.children[child.Location]
			require.True(t, ok, "unexpected child location %d", child.Location)
			pending = append(pending, pair{int(i), expectedChild})
		}
		require.Len(t, children, len(expected.children))
	}
	require.Len(t, seen, len(want))
}

func TestStacktraceTreeInsertBenchmarkWorkloads(t *testing.T) {
	for _, workload := range stacktraceInsertWorkloads() {
		t.Run(workload.name, func(t *testing.T) {
			build := stacktraceInsertBatch(workload.stacks, rand.New(rand.NewSource(0)).Perm(len(workload.stacks)))
			for _, capacity := range []int{1, len(stacktraceInsertReference(build))} {
				tree := NewStacktraceTree(capacity)
				for pass := 0; pass < 2; pass++ {
					for _, stack := range build {
						leaf := tree.Insert(stack, 1)
						path := tree.LookupLocations(nil, leaf)
						require.Len(t, path, len(stack))
						for i, location := range stack {
							require.Equal(t, uint64(location), path[i])
						}
					}
					requireStacktraceInsertReference(t, tree, build)
					replay := stacktraceReplayBatch(build, workload.wide)
					insertStacktraceBatch(tree, replay)
					requireStacktraceInsertReference(t, tree, append(slices.Clone(build), replay...))
					tree.Reset()
				}
			}
		})
	}
}
