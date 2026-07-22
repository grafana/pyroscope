package model

import (
	"math/rand"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Benchmarks reproducing the query-backend report aggregation cost: N
// serialized tree reports of the same service (heavily overlapping) merged
// into one tree. Tree().Total() forces finalization so deferred work is
// always measured.

var mergeBenchData = struct {
	once   sync.Once
	small  [][]byte // ~15 shards of a modest tree (worker-level aggregation)
	large  [][]byte // ~15 shards of a large tree (root-level aggregation)
	sizes  map[string]int
	fixmux sync.Mutex
	fix    [][]byte // real trees from testdata (diff fixtures)
}{}

func mergeBenchShards(b *testing.B, kind string) [][]byte {
	mergeBenchData.once.Do(func() {
		rng := rand.New(rand.NewSource(42))
		small := generateMergeTestStacks(rng, 8_000, 2_000, mkFunctionName)
		mergeBenchData.small = buildShardTreeBytes[FunctionName, FunctionNameI](rng, small, 15, -1, nil)
		large := generateMergeTestStacks(rng, 60_000, 8_000, mkFunctionName)
		mergeBenchData.large = buildShardTreeBytes[FunctionName, FunctionNameI](rng, large, 15, -1, nil)
	})
	switch kind {
	case "small":
		return mergeBenchData.small
	case "large":
		return mergeBenchData.large
	default:
		b.Fatalf("unknown shard kind %q", kind)
		return nil
	}
}

func fixtureTreeBytes(b *testing.B) [][]byte {
	mergeBenchData.fixmux.Lock()
	defer mergeBenchData.fixmux.Unlock()
	if mergeBenchData.fix == nil {
		left, err := os.ReadFile("testdata/diff_left_tree.bin")
		require.NoError(b, err)
		right, err := os.ReadFile("testdata/diff_right_tree.bin")
		require.NoError(b, err)
		mergeBenchData.fix = [][]byte{left, right}
	}
	return mergeBenchData.fix
}

func benchmarkMergeTreeBytes(b *testing.B, shards [][]byte) {
	var total int
	for _, s := range shards {
		total += len(s)
	}
	b.SetBytes(int64(total))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := NewTreeMerger[FunctionName, FunctionNameI]()
		for _, s := range shards {
			if err := m.MergeTreeBytes(s); err != nil {
				b.Fatal(err)
			}
		}
		if m.Tree().Total() == 0 {
			b.Fatal("empty merge result")
		}
	}
}

func Benchmark_MergeTreeBytes_Sharded_Small(b *testing.B) {
	benchmarkMergeTreeBytes(b, mergeBenchShards(b, "small"))
}

func Benchmark_MergeTreeBytes_Sharded_Large(b *testing.B) {
	benchmarkMergeTreeBytes(b, mergeBenchShards(b, "large"))
}

func Benchmark_MergeTreeBytes_Fixture(b *testing.B) {
	benchmarkMergeTreeBytes(b, fixtureTreeBytes(b))
}

func Benchmark_UnmarshalTree_Fixture(b *testing.B) {
	data := fixtureTreeBytes(b)[0]
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t, err := UnmarshalTree[FunctionName, FunctionNameI](data)
		if err != nil {
			b.Fatal(err)
		}
		if t.Total() == 0 {
			b.Fatal("empty tree")
		}
	}
}

func Benchmark_UnmarshalTree_Sharded_Large(b *testing.B) {
	data := mergeBenchShards(b, "large")[0]
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t, err := UnmarshalTree[FunctionName, FunctionNameI](data)
		if err != nil {
			b.Fatal(err)
		}
		if t.Total() == 0 {
			b.Fatal("empty tree")
		}
	}
}
