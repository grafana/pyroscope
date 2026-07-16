package model

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// The tests in this file assert that TreeMerger.MergeTreeBytes is equivalent
// to the reference algorithm it replaces: unmarshal every serialized tree and
// merge the in-memory trees one by one. UnmarshalTree and Tree.Merge keep
// their semantics, so the reference is expressed through them.

type mergeTestStack[N NodeName] struct {
	stack []N
	value int64
}

// generateMergeTestStacks produces a universe of stack traces with a
// realistic call-tree shape: new stacks reuse a random prefix of an existing
// stack and append a fresh suffix, so prefixes are heavily shared and the
// resulting tree is deep and wide, like a real service profile.
func generateMergeTestStacks[N NodeName](rng *rand.Rand, numStacks, corpusSize int, mkName func(int) N) []mergeTestStack[N] {
	corpus := make([]N, corpusSize)
	for i := range corpus {
		corpus[i] = mkName(i)
	}
	pick := func() N { return corpus[rng.Intn(len(corpus))] }

	stacks := make([]mergeTestStack[N], 0, numStacks)
	stacks = append(stacks, mergeTestStack[N]{
		stack: []N{mkName(0), mkName(1)},
		value: 1,
	})
	for len(stacks) < numStacks {
		base := stacks[rng.Intn(len(stacks))].stack
		prefix := base[:rng.Intn(len(base))+1]
		if len(prefix) > 80 {
			prefix = prefix[:80]
		}
		stack := make([]N, 0, len(prefix)+8)
		stack = append(stack, prefix...)
		for i, n := 0, 1+rng.Intn(8); i < n; i++ {
			stack = append(stack, pick())
		}
		value := rng.Int63n(10_000) + 1
		if rng.Intn(50) == 0 {
			value *= 1000
		}
		stacks = append(stacks, mergeTestStack[N]{stack: stack, value: value})
	}
	return stacks
}

func mkFunctionName(i int) FunctionName {
	switch i % 3 {
	case 0:
		return FunctionName(fmt.Sprintf("github.com/grafana/pyroscope/pkg/pkg%03d.(*worker%02d).processBatchItems%04d", i%97, i%13, i))
	case 1:
		return FunctionName(fmt.Sprintf("com.example.rt.surge.delivery.internal.dispatch.Dispatcher%02d.handleRequestWithRetries%05d", i%31, i))
	default:
		return FunctionName(fmt.Sprintf("net/http.(*conn%02d).serve.func%d", i%7, i))
	}
}

func mkLocationRefName(i int) LocationRefName { return LocationRefName(i) }

// buildShardTreeBytes distributes the stacks over overlapping shards
// (mimicking blocks of the same service) and returns each shard tree
// serialized with MarshalTruncate.
// keepName is required by LocationRefName marshaling (the symbols to retain);
// FunctionName trees ignore it and take nil.
func buildShardTreeBytes[N NodeName, I NodeNameI[N]](
	rng *rand.Rand,
	stacks []mergeTestStack[N],
	numShards int,
	maxNodes int64,
	keepName func(N) N,
) [][]byte {
	trees := make([]*Tree[N, I], numShards)
	for i := range trees {
		trees[i] = new(Tree[N, I])
	}
	for _, s := range stacks {
		assigned := false
		for i := range trees {
			if rng.Float64() < 0.6 {
				trees[i].InsertStack(s.value+rng.Int63n(100), s.stack...)
				assigned = true
			}
		}
		if !assigned {
			trees[rng.Intn(numShards)].InsertStack(s.value, s.stack...)
		}
	}
	shards := make([][]byte, numShards)
	for i, t := range trees {
		shards[i] = t.Bytes(maxNodes, keepName)
	}
	return shards
}

// referenceMergeTreeBytes is the original merge algorithm: unmarshal each
// serialized tree, then merge the in-memory trees.
func referenceMergeTreeBytes[N NodeName, I NodeNameI[N]](tb testing.TB, shards [][]byte, format func(N) N) *Tree[N, I] {
	dst := new(Tree[N, I])
	for _, b := range shards {
		src, err := UnmarshalTree[N, I](b)
		require.NoError(tb, err)
		if format != nil {
			src.FormatNodeNames(format)
		}
		dst.Merge(src)
	}
	return dst
}

// requireTreesEqual compares two trees node by node, including totals.
// Children are expected in name order, which every construction path
// (InsertStack, Merge, UnmarshalTree, MergeTreeBytes) maintains.
func requireTreesEqual[N NodeName, I NodeNameI[N]](tb testing.TB, expected, actual *Tree[N, I]) {
	type pair struct{ e, a *node[N] }
	stack := []pair{{
		e: &node[N]{children: expected.root},
		a: &node[N]{children: actual.root},
	}}
	var visited int
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		require.Equal(tb, p.e.name, p.a.name)
		require.Equal(tb, p.e.self, p.a.self, "self mismatch at node %v", p.e.name)
		require.Equal(tb, p.e.total, p.a.total, "total mismatch at node %v", p.e.name)
		require.Equal(tb, len(p.e.children), len(p.a.children), "children count mismatch at node %v", p.e.name)
		for i := range p.e.children {
			stack = append(stack, pair{e: p.e.children[i], a: p.a.children[i]})
		}
		visited++
	}
	require.Greater(tb, visited, 0)
}

func Test_TreeMerger_MergeTreeBytes_Sharded(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	stacks := generateMergeTestStacks(rng, 20_000, 4_000, mkFunctionName)
	shards := buildShardTreeBytes[FunctionName, FunctionNameI](rng, stacks, 15, -1, nil)

	expected := referenceMergeTreeBytes[FunctionName, FunctionNameI](t, shards, nil)

	m := NewTreeMerger[FunctionName, FunctionNameI]()
	for _, b := range shards {
		require.NoError(t, m.MergeTreeBytes(b))
	}
	require.False(t, m.IsEmpty())
	requireTreesEqual(t, expected, m.Tree())
}

func Test_TreeMerger_MergeTreeBytes_TruncatedShards(t *testing.T) {
	// Truncated shard trees contain "other" placeholder nodes, which must
	// merge like regular nodes.
	rng := rand.New(rand.NewSource(2))
	stacks := generateMergeTestStacks(rng, 5_000, 1_000, mkFunctionName)
	shards := buildShardTreeBytes[FunctionName, FunctionNameI](rng, stacks, 8, 500, nil)

	expected := referenceMergeTreeBytes[FunctionName, FunctionNameI](t, shards, nil)

	m := NewTreeMerger[FunctionName, FunctionNameI]()
	for _, b := range shards {
		require.NoError(t, m.MergeTreeBytes(b))
	}
	requireTreesEqual(t, expected, m.Tree())
}

func Test_TreeMerger_MergeTreeBytes_Concurrent(t *testing.T) {
	// The aggregator merges reports concurrently; exercise the same pattern
	// so the race detector can catch locking regressions.
	rng := rand.New(rand.NewSource(3))
	stacks := generateMergeTestStacks(rng, 10_000, 2_000, mkFunctionName)
	shards := buildShardTreeBytes[FunctionName, FunctionNameI](rng, stacks, 15, -1, nil)

	expected := referenceMergeTreeBytes[FunctionName, FunctionNameI](t, shards, nil)

	m := NewTreeMerger[FunctionName, FunctionNameI]()
	var g errgroup.Group
	for _, b := range shards {
		g.Go(func() error { return m.MergeTreeBytes(b) })
	}
	require.NoError(t, g.Wait())
	requireTreesEqual(t, expected, m.Tree())
}

func Test_TreeMerger_MixedMergeTreeAndBytes(t *testing.T) {
	// MergeTree (in-memory) and MergeTreeBytes may be used on the same
	// merger; totals must come out right regardless of the interleaving.
	rng := rand.New(rand.NewSource(4))
	stacks := generateMergeTestStacks(rng, 2_000, 500, mkFunctionName)
	shards := buildShardTreeBytes[FunctionName, FunctionNameI](rng, stacks, 4, -1, nil)

	expected := referenceMergeTreeBytes[FunctionName, FunctionNameI](t, shards, nil)

	for _, inMemory := range []int{0, 1, 3} {
		m := NewTreeMerger[FunctionName, FunctionNameI]()
		for i, b := range shards {
			if i == inMemory {
				src, err := UnmarshalTree[FunctionName, FunctionNameI](b)
				require.NoError(t, err)
				m.MergeTree(src)
				continue
			}
			require.NoError(t, m.MergeTreeBytes(b))
		}
		requireTreesEqual(t, expected, m.Tree())
	}
}

func Test_TreeMerger_FormatNodeNames(t *testing.T) {
	// The FullSymbols path merges LocationRefName trees while remapping
	// references through the symbol merger. The remap function may map
	// distinct names to the same value; such siblings must be merged,
	// matching FormatNodeNames+Fix semantics of the reference.
	rng := rand.New(rand.NewSource(5))
	stacks := generateMergeTestStacks(rng, 5_000, 1_000, mkLocationRefName)
	shards := buildShardTreeBytes[LocationRefName, LocationRefNameI](rng, stacks, 6, -1, func(n LocationRefName) LocationRefName { return n })

	format := func(n LocationRefName) LocationRefName { return n / 2 }
	expected := referenceMergeTreeBytes[LocationRefName, LocationRefNameI](t, shards, format)

	m := NewTreeMerger[LocationRefName, LocationRefNameI]()
	for _, b := range shards {
		require.NoError(t, m.MergeTreeBytes(b, WithTreeMergeFormatNodeNames(format)))
	}
	requireTreesEqual(t, expected, m.Tree())
}

func Test_TreeMerger_SingleShard(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	stacks := generateMergeTestStacks(rng, 1_000, 300, mkFunctionName)
	shards := buildShardTreeBytes[FunctionName, FunctionNameI](rng, stacks, 1, -1, nil)

	expected, err := UnmarshalTree[FunctionName, FunctionNameI](shards[0])
	require.NoError(t, err)

	m := NewTreeMerger[FunctionName, FunctionNameI]()
	require.NoError(t, m.MergeTreeBytes(shards[0]))
	requireTreesEqual(t, expected, m.Tree())
}

func Test_TreeMerger_EmptyAndMalformed(t *testing.T) {
	t.Run("empty bytes are a no-op", func(t *testing.T) {
		m := NewTreeMerger[FunctionName, FunctionNameI]()
		require.NoError(t, m.MergeTreeBytes(nil))
		require.NoError(t, m.MergeTreeBytes([]byte{}))
		require.Equal(t, int64(0), m.Tree().Total())
	})

	t.Run("empty tree bytes merged into existing tree", func(t *testing.T) {
		tree := new(FunctionNameTree)
		tree.InsertStack(1, "a", "b")
		b := tree.Bytes(-1, nil)

		empty := new(FunctionNameTree).Bytes(-1, nil)

		m := NewTreeMerger[FunctionName, FunctionNameI]()
		require.NoError(t, m.MergeTreeBytes(b))
		require.NoError(t, m.MergeTreeBytes(empty))
		require.Equal(t, int64(1), m.Tree().Total())
	})

	t.Run("malformed bytes return an error", func(t *testing.T) {
		m := NewTreeMerger[FunctionName, FunctionNameI]()
		require.Error(t, m.MergeTreeBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}))
	})
}
