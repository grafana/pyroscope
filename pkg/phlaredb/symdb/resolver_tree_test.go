package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_memory_Resolver_ResolveTree(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)

	t.Run("default", func(t *testing.T) {
		r := NewResolver(context.Background(), s.db)
		defer r.Release()
		r.AddSamples(0, s.indexed[0][0].Samples)
		resolved, err := r.Tree()
		require.NoError(t, err)
		require.Equal(t, expectedFingerprint, treeFingerprint(resolved))
	})

	for _, tc := range []struct {
		name            string
		callsite        []string
		stacktraceCount int
		total           int
	}{
		{
			name: "multiple stacks",
			callsite: []string{
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*scrapeLoop).run",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*Target).report",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*scrapeLoop).scrape",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*pprofWriter).writeProfile",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*cache).writeProfiles",
				"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Insert",
			},
			stacktraceCount: 4,
			total:           2752628,
		},
		{
			name: "single stack",
			callsite: []string{
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*scrapeLoop).run",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*Target).report",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*scrapeLoop).scrape",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*pprofWriter).writeProfile",
				"github.com/pyroscope-io/pyroscope/pkg/scrape.(*cache).writeProfiles",
				"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Insert",
				"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*trieNode).findNodeAt",
				"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.newTrieNode",
			},
			stacktraceCount: 1,
			total:           417817,
		},
		{
			name: "no match",
			callsite: []string{
				"github.com/no-match/no-match.main",
			},
			stacktraceCount: 0,
			total:           0,
		},
	} {
		t.Run("with stack trace selector/"+tc.name, func(t *testing.T) {
			sts := &typesv1.StackTraceSelector{
				CallSite: make([]*typesv1.Location, len(tc.callsite)),
			}
			for i, name := range tc.callsite {
				sts.CallSite[i] = &typesv1.Location{
					Name: name,
				}
			}

			r := NewResolver(context.Background(), s.db, WithResolverStackTraceSelector(sts), WithResolverMaxNodes(10))
			defer r.Release()
			r.AddSamples(0, s.indexed[0][0].Samples)
			resolved, err := r.Tree()
			require.NoError(t, err)

			stacktraceCount := 0
			total := 0

			resolved.IterateStacks(func(name string, self int64, stack []string) {
				stacktraceCount++
				total += int(self)

				prefix := make([]string, len(tc.callsite))
				for i := range prefix {
					prefix[i] = stack[len(stack)-1-i]
				}
				require.Equal(t, tc.callsite, prefix, "stack prefix doesn't match")
			})
			assert.Equal(t, tc.stacktraceCount, stacktraceCount)
			assert.Equal(t, tc.total, total)
		})
	}
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
	expectedTruncatedTree := `.
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

	for _, tc := range []struct {
		name     string
		selector *typesv1.StackTraceSelector
		expected string
	}{
		{
			name:     "without selection",
			selector: nil,
			expected: expectedTruncatedTree,
		},
		{
			name: "with common prefix selection",
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{
					{Name: "a"},
					{Name: "b"},
					{Name: "c"},
				},
			},
			expected: expectedTruncatedTree,
		},
		{
			name: "with focus on truncated callsite last shown",
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{
					{Name: "a"},
					{Name: "b"},
					{Name: "c"},
					{Name: "f1"},
				},
			},
			expected: `.
└── a: self 0 total 2
    └── b: self 0 total 2
        └── c: self 0 total 2
            └── f1: self 1 total 2
                └── f2: self 1 total 1
`,
		},
		{
			name: "with focus on truncated callsite",
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{
					{Name: "a"},
					{Name: "b"},
					{Name: "c"},
					{Name: "f1"},
					{Name: "f2"},
				},
			},
			expected: `.
└── a: self 0 total 1
    └── b: self 0 total 1
        └── c: self 0 total 1
            └── f1: self 0 total 1
                └── f2: self 1 total 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appender := NewSampleAppender()
			appender.AppendMany(expectedSamples.StacktraceIDs, expectedSamples.Values)
			ranges := iterator.SplitStacktraceIDRanges(appender)
			resolved, err := buildTreeFromParentPointerTrees(context.Background(), ranges, symbols, maxNodes, SelectStackTraces(symbols, tc.selector))
			require.NoError(t, err)

			require.Equal(t, tc.expected, resolved.String())
		})
	}
}
