package symdb

import (
	"fmt"
	"math/rand"
	"testing"
)

// insertScanOnly is the insert without child indexing.
func (t *stacktraceTree) insertScanOnly(refs []uint64) uint32 {
	var parent int32
	for j := len(refs) - 1; j >= 0; j-- {
		r := int32(refs[j])
		i := t.nodes[parent].fc
		for i != sentinel && t.nodes[i].r != r {
			i = t.nodes[i].ns
		}
		if i == sentinel {
			i = t.newChild(parent, r)
		}
		parent = i
	}
	return uint32(parent)
}

// uniformStacks: every frame is drawn uniformly from fanout callees, so wide
// nodes have many children and little reuse; consecutive stacks share prefixes.
func uniformStacks(seed int64, n, depth, fanout int) [][]uint64 {
	rnd := rand.New(rand.NewSource(seed))
	stacks := make([][]uint64, 0, n)
	prefix := make([]uint64, 0, depth)
	for len(stacks) < n {
		prefix = prefix[:rnd.Intn(len(prefix)+1)]
		for len(prefix) < depth {
			prefix = append(prefix, uint64(rnd.Intn(fanout))+1)
		}
		s := make([]uint64, depth)
		for k := range prefix {
			s[depth-1-k] = prefix[k]
		}
		stacks = append(stacks, s)
	}
	return stacks
}

// mergeStacks imitates compaction: sources are inserted one after another;
// frames come from a hot core shared by all sources (40%) or from frames
// unique to the source (60%).
func mergeStacks(seed int64, sources, perSource, depth int) [][]uint64 {
	rnd := rand.New(rand.NewSource(seed))
	zipf := rand.NewZipf(rnd, 1.1, 1, 999)
	var stacks [][]uint64
	for src := 0; src < sources; src++ {
		base := uint64(1_000_000 * (src + 1))
		prefix := make([]uint64, 0, depth)
		for k := 0; k < perSource; k++ {
			prefix = prefix[:rnd.Intn(len(prefix)+1)]
			for len(prefix) < depth {
				if rnd.Intn(10) < 4 {
					prefix = append(prefix, zipf.Uint64()+1)
				} else {
					prefix = append(prefix, base+uint64(rnd.Intn(20000)))
				}
			}
			s := make([]uint64, depth)
			for i := range prefix {
				s[depth-1-i] = prefix[i]
			}
			stacks = append(stacks, s)
		}
	}
	return stacks
}

func Benchmark_stacktrace_tree_insert_variants(b *testing.B) {
	workloads := []struct {
		name   string
		stacks [][]uint64
	}{
		{"skewed", skewedStacks(1, 200000, 40, 5000)},
		{"uniform", uniformStacks(1, 200000, 30, 3000)},
		{"merge", mergeStacks(1, 10, 20000, 30)},
	}
	variants := []struct {
		name      string
		threshold int
		insert    func(*stacktraceTree, []uint64) uint32
	}{
		{"scan", 0, (*stacktraceTree).insertScanOnly},
		{"index", wideNodeScan, (*stacktraceTree).insert},
	}
	for _, w := range workloads {
		for _, v := range variants {
			b.Run(fmt.Sprintf("%s/%s", w.name, v.name), func(b *testing.B) {
				b.ReportAllocs()
				var nodes int
				for i := 0; i < b.N; i++ {
					x := newStacktraceTree(0)
					x.wideThreshold = v.threshold
					for _, s := range w.stacks {
						v.insert(x, s)
					}
					nodes = len(x.nodes)
				}
				b.ReportMetric(float64(nodes), "nodes")
			})
		}
	}
}
