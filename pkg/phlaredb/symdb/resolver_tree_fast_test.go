package symdb

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

// forEachCallTreeMode runs the test with call trees built sequentially and
// concurrently.
func forEachCallTreeMode(t *testing.T, fn func(t *testing.T)) {
	defer func(n int) { parallelCallTreeMinNodes = n }(parallelCallTreeMinNodes)
	for _, n := range []int{1 << 30, 0} {
		parallelCallTreeMinNodes = n
		t.Run(fmt.Sprintf("parallel_min_nodes=%d", n), fn)
	}
}

func Test_buildTruncatedCallTree_matches_parent_pointer_tree_copies(t *testing.T) {
	forEachCallTreeMode(t, testBuildTruncatedCallTree)
}

func Test_buildCallTree_matches_stacktrace_inserts(t *testing.T) {
	forEachCallTreeMode(t, testBuildCallTree)
}

func testBuildTruncatedCallTree(t *testing.T) {
	for _, f := range []string{"testdata/profile.pb.gz", "testdata/big-profile.pb.gz"} {
		t.Run(f, func(t *testing.T) {
			s := newBlockSuite(t, [][]string{{f}})
			defer s.teardown()
			p, err := s.reader.Partition(context.Background(), 0)
			require.NoError(t, err)
			defer p.Release()
			symbols := p.Symbols()
			lookup := func(i int32) model.FunctionName { return model.FunctionName(symbols.Strings[i]) }
			iterator := symbols.Stacktraces.(StacktraceIDRangeIterator)

			for _, samples := range s.indexed[0] {
				for _, maxNodes := range []int64{0, 1, 16, 100, 1 << 10, 16 << 10, 1 << 20} {
					appender := NewSampleAppender()
					appender.AppendMany(samples.Samples.StacktraceIDs, samples.Samples.Values)

					expected, err := buildTreeFromParentPointerTrees[model.FunctionName, model.FunctionNameI](
						context.Background(), iterator.SplitStacktraceIDRanges(appender), symbols, maxNodes, nil, lookup)
					require.NoError(t, err)

					c, ok := buildTruncatedCallTree(symbols, appender, maxNodes)
					require.True(t, ok)
					actual := model.TreeFromStacktraceTree[model.FunctionName, model.FunctionNameI](stacktraceTreeFromCallTree(c), maxNodes, lookup)
					require.Equal(t, expected.Bytes(maxNodes, nil), actual.Bytes(maxNodes, nil), "max nodes %d", maxNodes)

					expected, err = buildTreeFromParentPointerTrees[model.FunctionName, model.FunctionNameI](
						context.Background(), iterator.SplitStacktraceIDRanges(appender), symbols, maxNodes, nil, lookup)
					require.NoError(t, err)
					f := model.NewFlatTree()
					f.MergeCallTree(c, maxNodes)
					require.Equal(t, expected.Bytes(maxNodes, nil), f.Bytes(maxNodes), "max nodes %d", maxNodes)
				}
			}
		})
	}
}

func testBuildCallTree(t *testing.T) {
	for _, f := range []string{"testdata/profile.pb.gz", "testdata/big-profile.pb.gz"} {
		t.Run(f, func(t *testing.T) {
			s := newBlockSuite(t, [][]string{{f}})
			defer s.teardown()
			p, err := s.reader.Partition(context.Background(), 0)
			require.NoError(t, err)
			defer p.Release()
			symbols := p.Symbols()
			lookup := func(i int32) model.FunctionName { return model.FunctionName(symbols.Strings[i]) }

			for _, samples := range s.indexed[0] {
				for _, maxNodes := range []int64{0, 16, 1 << 10, 16 << 10, 1 << 20} {
					appender := NewSampleAppender()
					appender.AppendMany(samples.Samples.StacktraceIDs, samples.Samples.Values)

					c, ok := buildCallTree(symbols, appender)
					require.True(t, ok)
					actual := model.TreeFromStacktraceTree[model.FunctionName, model.FunctionNameI](stacktraceTreeFromCallTree(c), maxNodes, lookup)

					// The call tree merged directly is serialized
					// as the converted tree is.
					f := model.NewFlatTree()
					f.MergeCallTree(c, maxNodes)
					require.Equal(t, actual.Bytes(maxNodes, nil), f.Bytes(maxNodes), "max nodes %d", maxNodes)
					actual = model.TreeFromStacktraceTree[model.FunctionName, model.FunctionNameI](stacktraceTreeFromCallTree(c), maxNodes, lookup)

					expected, err := buildTreeFromStacktraces[model.FunctionName, model.FunctionNameI](
						context.Background(), symbols, appender, maxNodes, nil, lookup)
					require.NoError(t, err)

					require.Equal(t, expected.Total(), actual.Total())
					require.Equal(t, expected.Bytes(maxNodes, nil), actual.Bytes(maxNodes, nil), "max nodes %d", maxNodes)
				}
			}
		})
	}
}

type countedStacktraces struct {
	StacktraceResolver
	n int
}

func (s countedStacktraces) stacktraceCount() int { return s.n }

func Test_denseSamples(t *testing.T) {
	appender := func(stacks int) *SampleAppender {
		a := NewSampleAppender()
		for i := 1; i <= stacks; i++ {
			a.Append(uint32(i), 1)
		}
		return a
	}
	symbols := &Symbols{Stacktraces: countedStacktraces{n: 1600}}
	require.True(t, denseSamples(symbols, appender(1600)))
	require.True(t, denseSamples(symbols, appender(100)))
	require.False(t, denseSamples(symbols, appender(99)))
	require.False(t, denseSamples(symbols, appender(0)))
	require.False(t, denseSamples(&Symbols{}, appender(1600)))
}
