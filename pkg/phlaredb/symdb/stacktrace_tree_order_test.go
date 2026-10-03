package symdb

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// skewedStacks draws stacks from a call tree with Zipf-distributed fan-out (a few hot callees
// per frame, a long tail of cold ones); consecutive stacks share prefixes like one profile's.
func skewedStacks(seed int64, n, depth, fanout int) [][]uint64 {
	rnd := rand.New(rand.NewSource(seed))
	zipf := rand.NewZipf(rnd, 1.2, 1, uint64(fanout-1))
	stacks := make([][]uint64, 0, n)
	prefix := make([]uint64, 0, depth)
	for len(stacks) < n {
		prefix = prefix[:rnd.Intn(len(prefix)+1)]
		for len(prefix) < depth {
			prefix = append(prefix, zipf.Uint64()+1)
		}
		s := make([]uint64, depth)
		for k := range prefix {
			s[depth-1-k] = prefix[k] // Leaf first, as stack traces are stored.
		}
		stacks = append(stacks, s)
	}
	return stacks
}

func Test_stacktrace_tree_insert_skewed(t *testing.T) {
	stacks := skewedStacks(1, 50000, 40, 2000)
	x := newStacktraceTree(0)
	ids := make([]uint32, len(stacks))
	for i, s := range stacks {
		ids[i] = x.insert(s)
	}
	var dst []int32
	for i, s := range stacks {
		require.Equal(t, ids[i], x.insert(s))
		dst = x.resolve(dst, ids[i])
		require.Len(t, dst, len(s))
		for k := range s {
			require.Equal(t, int32(s[k]), dst[k])
		}
	}
}

func Benchmark_stacktrace_tree_insert_skewed(b *testing.B) {
	stacks := skewedStacks(1, 200000, 40, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := newStacktraceTree(0)
		for _, s := range stacks {
			x.insert(s)
		}
	}
}
