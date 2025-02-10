package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_memory_Resolver_ResolveTree(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Tree()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, treeFingerprint(resolved))
}

func Test_block_Resolver_ResolveTree(t *testing.T) {
	s := newBlockSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	defer s.teardown()
	expectedFingerprint := pprofFingerprint(s.profiles[0], 1)
	r := NewResolver(context.Background(), s.reader)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][1].Samples)
	resolved, err := r.Tree()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, treeFingerprint(resolved))
}

func Benchmark_Resolver_ResolveTree_Small(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolveTree(s.db, samples, 0))
	b.Run("1K", benchmarkResolverResolveTree(s.db, samples, 1<<10))
	b.Run("8K", benchmarkResolverResolveTree(s.db, samples, 8<<10))
}

func Benchmark_Resolver_ResolveTree_Big(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/big-profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolveTree(s.db, samples, 0))
	b.Run("8K", benchmarkResolverResolveTree(s.db, samples, 8<<10))
	b.Run("16K", benchmarkResolverResolveTree(s.db, samples, 16<<10))
	b.Run("32K", benchmarkResolverResolveTree(s.db, samples, 32<<10))
	b.Run("64K", benchmarkResolverResolveTree(s.db, samples, 64<<10))
}

func benchmarkResolverResolveTree(sym SymbolsReader, samples v1.Samples, n int64) func(b *testing.B) {
	return func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := NewResolver(context.Background(), sym, WithResolverMaxNodes(n))
			r.AddSamples(0, samples)
			_, _ = r.Tree()
		}
	}
}

func Test_memory_Resolver_ResolveTree_copied_nodes(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/big-profile.pb.gz"}})
	samples := s.indexed[0][0].Samples

	resolve := func(options ...ResolverOption) (nodes, total int64) {
		r := NewResolver(context.Background(), s.db, options...)
		defer r.Release()
		r.AddSamples(0, samples)
		resolved, err := r.Tree()
		require.NoError(t, err)
		resolved.FormatNodeNames(func(s string) string {
			nodes++
			return s
		})
		return nodes, resolved.Total()
	}

	const maxNodes int64 = 16 << 10
	nodesFull, totalFull := resolve()
	nodesTrunc, totalTrunc := resolve(WithResolverMaxNodes(maxNodes))
	// The only reason we perform this assertion is to make sure that
	// truncation did take place, and the number of nodes is close to
	// the target (we actually keep all nodes with top 16K values).
	assert.Equal(t, int64(1585462), nodesFull)
	assert.Equal(t, int64(22461), nodesTrunc)
	require.Equal(t, totalFull, totalTrunc)
}

func Test_buildTreeFromParentPointerTrees(t *testing.T) {
	// The profile has the following samples:
	//
	//	a b c f f1 f2 f3 f4 f5
	//	1 2 3 4 5  6  7  8  9
	//
	//	4: a b c f
	//	5: a b c f1
	//	6: a b c f1 f2
	//	8: a b c f3 f4
	//	9: a b c f3 f4 f5
	//
	expectedSamples := v1.Samples{
		StacktraceIDs: []uint32{4, 5, 6, 8, 9},
		Values:        []uint64{1, 1, 1, 1, 1},
	}

	// After the truncation, we expect to see the following tree
	// (function f, f2, and f5 are replaced with "other"):
	const maxNodes = 6
	expectedTree := `.
└── a: self 0 total 5
    └── b: self 0 total 5
        └── c: self 0 total 5
            ├── f1: self 1 total 2
            │   └── other: self 1 total 1
            ├── f3: self 0 total 2
            │   └── f4: self 1 total 2
            │       └── other: self 1 total 1
            └── other: self 1 total 1
`

	p := &profilev1.Profile{
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{4, 3, 2, 1}, Value: []int64{1}},
			{LocationId: []uint64{5, 3, 2, 1}, Value: []int64{1}},
			{LocationId: []uint64{6, 5, 3, 2, 1}, Value: []int64{1}},
			{LocationId: []uint64{8, 7, 3, 2, 1}, Value: []int64{1}},
			{LocationId: []uint64{9, 8, 7, 3, 2, 1}, Value: []int64{1}},
		},
		StringTable: []string{
			"", "a", "b", "c", "f", "f1", "f2", "f3", "f4", "f5",
		},
	}

	names := uint64(len(p.StringTable))
	for i := uint64(1); i < names; i++ {
		p.Location = append(p.Location, &profilev1.Location{
			Id: i, Line: []*profilev1.Line{{FunctionId: i}},
		})
		p.Function = append(p.Function, &profilev1.Function{
			Id: i, Name: int64(i),
		})
	}

	s := newMemSuite(t, nil)
	const partition = 0
	indexed := s.db.WriteProfileSymbols(partition, p)
	assert.Equal(t, expectedSamples, indexed[partition].Samples)
	b := blockSuite{memSuite: s}
	b.flush()
	pr, err := b.reader.Partition(context.Background(), partition)
	require.NoError(t, err)
	symbols := pr.Symbols()
	iterator, ok := symbols.Stacktraces.(StacktraceIDRangeIterator)
	require.True(t, ok)

	appender := NewSampleAppender()
	appender.AppendMany(expectedSamples.StacktraceIDs, expectedSamples.Values)
	ranges := iterator.SplitStacktraceIDRanges(appender)
	resolved, err := buildTreeFromParentPointerTrees(context.Background(), ranges, symbols, maxNodes)
	require.NoError(t, err)

	require.Equal(t, expectedTree, resolved.String())
}
