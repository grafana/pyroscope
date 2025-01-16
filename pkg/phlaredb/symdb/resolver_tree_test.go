package symdb

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/experiment/symbolizer"
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

func Test_buildTree_Symbolization(t *testing.T) {
	t.Run("no symbolizer configured", func(t *testing.T) {
		stringTable := []string{
			"",
			"test",
			"hex_address",
		}

		p := &profilev1.Profile{
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{100}},
			},
			Location: []*profilev1.Location{
				{
					Id:        1,
					MappingId: 1,
					Address:   0x1234,
					Line: []*profilev1.Line{
						{
							FunctionId: 1,
							Line:       0,
						},
					},
				},
			},
			Function: []*profilev1.Function{
				{
					Id:         1,
					Name:       2,
					SystemName: 2,
					Filename:   1,
				},
			},
			Mapping: []*profilev1.Mapping{
				{
					Id:          1,
					MemoryStart: 0x1000,
					MemoryLimit: 0x2000,
					BuildId:     int64(1),
					Filename:    1,
				},
			},
			StringTable: stringTable,
			SampleType: []*profilev1.ValueType{
				{Type: 1, Unit: 1},
			},
		}

		s := newMemSuite(t, nil) // Start with empty suite
		const partition = 0
		indexed := s.db.WriteProfileSymbols(partition, p)

		// Get symbols through PartitionWriter
		partitionWriter := s.db.PartitionWriter(partition)
		symbols := partitionWriter.Symbols()
		symbols.Symbolizer = nil

		appender := NewSampleAppender()
		appender.AppendMany(indexed[partition].Samples.StacktraceIDs, indexed[partition].Samples.Values)

		tree, err := buildTree(context.Background(), symbols, appender, 0)
		require.NoError(t, err)

		require.NotEmpty(t, tree.String())
		require.Contains(t, tree.String(), "hex_address")
		require.Equal(t, uint64(4660), symbols.Locations[0].Address)
		require.Len(t, symbols.Locations[0].Line, 1)
	})

	t.Run("with symbolizer configured", func(t *testing.T) {
		stringTable := []string{
			"",
			"test",
			"hex_address",
		}

		p := &profilev1.Profile{
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{100}},
			},
			Location: []*profilev1.Location{{
				Id:        1,
				MappingId: 1,
				Address:   0x3c5a,
				// No Line info - this is what should get symbolized
			},
			},
			Mapping: []*profilev1.Mapping{
				{
					Id:          1,
					MemoryStart: 0x1000,
					MemoryLimit: 0x2000,
					BuildId:     int64(1),
					Filename:    1,
				},
			},
			StringTable: stringTable,
			SampleType: []*profilev1.ValueType{
				{Type: 1, Unit: 1},
			},
		}

		s := newMemSuite(t, nil)
		const partition = 0
		indexed := s.db.WriteProfileSymbols(partition, p)

		partitionWriter := s.db.PartitionWriter(partition)
		symbols := partitionWriter.Symbols()

		mockClient := &mockDebuginfodClient{
			fetchFunc: func(buildID string) (string, error) {
				return "testdata/unsymbolized.debug", nil
			},
		}
		sym := symbolizer.NewSymbolizer(mockClient)
		symbols.SetSymbolizer(sym)

		appender := NewSampleAppender()
		appender.AppendMany(indexed[partition].Samples.StacktraceIDs, indexed[partition].Samples.Values)

		tree, err := buildTree(context.Background(), symbols, appender, 0)
		require.NoError(t, err)

		require.NotEmpty(t, tree.String())
		require.Contains(t, tree.String(), "fprintf")
		require.NotEmpty(t, symbols.Locations[0].Line)
	})

	t.Run("with symbolizer configured", func(t *testing.T) {
		stringTable := []string{
			"",
			"test",
			"hex_address",
		}

		p := &profilev1.Profile{
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{100}},
			},
			Location: []*profilev1.Location{{
				Id:        1,
				MappingId: 1,
				Address:   0x3c5a,
				// No Line info - this is what should get symbolized
			},
			},
			Mapping: []*profilev1.Mapping{
				{
					Id:          1,
					MemoryStart: 0x1000,
					MemoryLimit: 0x2000,
					BuildId:     int64(1),
					Filename:    1,
				},
			},
			StringTable: stringTable,
			SampleType: []*profilev1.ValueType{
				{Type: 1, Unit: 1},
			},
		}

		s := newMemSuite(t, nil)
		const partition = 0
		indexed := s.db.WriteProfileSymbols(partition, p)

		partitionWriter := s.db.PartitionWriter(partition)
		symbols := partitionWriter.Symbols()

		mockClient := &mockDebuginfodClient{
			fetchFunc: func(buildID string) (string, error) {
				return "", fmt.Errorf("symbolization failed")
			},
		}
		sym := symbolizer.NewSymbolizer(mockClient)
		symbols.SetSymbolizer(sym)

		appender := NewSampleAppender()
		appender.AppendMany(indexed[partition].Samples.StacktraceIDs, indexed[partition].Samples.Values)

		_, err := buildTree(context.Background(), symbols, appender, 0)
		require.NoError(t, err)
	})
}

type mockDebuginfodClient struct {
	fetchFunc func(buildID string) (string, error)
}

func (m *mockDebuginfodClient) FetchDebuginfo(buildID string) (string, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(buildID)
	}
	return "", nil
}
