package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

func Test_buildCallTree_matches_stacktrace_inserts(t *testing.T) {
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
