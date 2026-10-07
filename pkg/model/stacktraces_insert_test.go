package model_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// insertScan inserts by walking each parent's sibling list, as Insert does
// for nodes that are not wide, and returns the same tree as Insert.
func insertScan(t *model.StacktraceTree, locations []int32, value int64) int32 {
	var parent int32
	for j := len(locations) - 1; j >= 0; j-- {
		r := locations[j]
		last := int32(-1)
		i := t.Nodes[parent].FirstChild
		for i != -1 && t.Nodes[i].Location != r {
			last, i = i, t.Nodes[i].NextSibling
		}
		if i == -1 {
			i = int32(len(t.Nodes))
			t.Nodes = append(t.Nodes, model.StacktraceNode{
				Parent:      parent,
				FirstChild:  -1,
				NextSibling: -1,
				Location:    r,
			})
			if last == -1 {
				t.Nodes[parent].FirstChild = i
			} else {
				t.Nodes[last].NextSibling = i
			}
		}
		t.Nodes[i].Total += value
		parent = i
	}
	t.Nodes[parent].Value += value
	return parent
}

// wideStacks returns stacks whose root and second frames fan out widely, as
// in profiles of whole hosts where many stacks are cut short at the leaf.
func wideStacks(n int, seed int64) [][]int32 {
	rnd := rand.New(rand.NewSource(seed))
	stacks := make([][]int32, n)
	for i := range stacks {
		s := make([]int32, 1+rnd.Intn(12))
		for j := range s {
			s[j] = int32(rnd.Intn(64))
		}
		s[len(s)-1] = int32(rnd.Intn(5000)) // The root frame (stacks are leaf first).
		if len(s) > 1 {
			s[len(s)-2] = int32(rnd.Intn(500))
		}
		stacks[i] = s
	}
	return stacks
}

// narrowStacks returns deep stacks over few frames per level: sibling lists
// stay short and no node becomes wide.
func narrowStacks(n int, seed int64) [][]int32 {
	rnd := rand.New(rand.NewSource(seed))
	stacks := make([][]int32, n)
	for i := range stacks {
		s := make([]int32, 20+rnd.Intn(100))
		for j := range s {
			s[j] = int32(rnd.Intn(4))
		}
		stacks[i] = s
	}
	return stacks
}

func profileStacks(tb testing.TB) [][]int32 {
	p, err := pprof.OpenFile("../phlaredb/symdb/testdata/big-profile.pb.gz")
	require.NoError(tb, err)
	stacks := make([][]int32, len(p.Sample))
	for i, s := range p.Sample {
		stacks[i] = make([]int32, len(s.LocationId))
		for j, id := range s.LocationId {
			stacks[i][j] = int32(id)
		}
	}
	return stacks
}

func Test_StacktraceTree_Insert_matches_sibling_scan(t *testing.T) {
	for name, stacks := range map[string][][]int32{
		"wide":    wideStacks(50000, 1),
		"narrow":  narrowStacks(5000, 2),
		"profile": profileStacks(t),
	} {
		t.Run(name, func(t *testing.T) {
			scanned := model.NewStacktraceTree(0)
			indexed := model.NewStacktraceTree(0)
			for i, s := range stacks {
				v := int64(i%7 + 1)
				require.Equal(t, insertScan(scanned, s, v), indexed.Insert(s, v))
			}
			require.Equal(t, scanned.Nodes, indexed.Nodes)
		})
	}
}

func Test_StacktraceTree_Reset_clears_wide_index(t *testing.T) {
	tree := model.NewStacktraceTree(0)
	for _, s := range wideStacks(20000, 3) {
		tree.Insert(s, 1)
	}
	tree.Reset()
	fresh := model.NewStacktraceTree(0)
	for _, s := range wideStacks(20000, 4) {
		require.Equal(t, fresh.Insert(s, 1), tree.Insert(s, 1))
	}
	require.Equal(t, fresh.Nodes, tree.Nodes)
}

// Benchmark_StacktraceTree_Insert uses only the public API, so it runs
// unchanged on both sides of a comparison with benchstat.
func Benchmark_StacktraceTree_Insert(b *testing.B) {
	for _, bc := range []struct {
		name   string
		stacks [][]int32
	}{
		{"wide", wideStacks(200000, 5)},
		{"narrow", narrowStacks(50000, 6)},
		{"profile", profileStacks(b)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tree := model.NewStacktraceTree(len(bc.stacks))
				for _, s := range bc.stacks {
					tree.Insert(s, 1)
				}
			}
		})
	}
}

// fanoutStacks returns two-frame stacks whose root has width distinct
// children, visited with a skew towards the first ones as in real profiles.
func fanoutStacks(n, width int, seed int64) [][]int32 {
	rnd := rand.New(rand.NewSource(seed))
	zipf := rand.NewZipf(rnd, 1.2, 1, uint64(width-1))
	stacks := make([][]int32, n)
	for i := range stacks {
		stacks[i] = []int32{int32(rnd.Intn(16)), int32(zipf.Uint64())}
	}
	return stacks
}

// Benchmark_StacktraceTree_Insert_fanout shows where indexing the children of
// a node starts to pay off, by the number of children the node has.
func Benchmark_StacktraceTree_Insert_fanout(b *testing.B) {
	for _, width := range []int{8, 16, 32, 64, 128, 256, 1024, 4096} {
		stacks := fanoutStacks(200000, width, int64(width))
		b.Run(fmt.Sprintf("children=%d", width), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tree := model.NewStacktraceTree(len(stacks))
				for _, s := range stacks {
					tree.Insert(s, 1)
				}
			}
		})
	}
}
