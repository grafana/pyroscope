package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/pprof"
)

func Benchmark_stacktrace_tree_insert(b *testing.B) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		x := newStacktraceTree(0)
		for j := range p.Sample {
			x.insert(p.Sample[j].LocationId)
		}
	}
}
