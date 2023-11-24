package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_memory_Resolver_ResolvePprof(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Pprof(0)
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, pprofFingerprint(resolved, 0))
}

func Test_block_Resolver_ResolvePprof_multiple_partitions(t *testing.T) {
	s := newBlockSuite(t, [][]string{
		{"testdata/profile.pb.gz"},
		{"testdata/profile.pb.gz"},
	})
	defer s.teardown()
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	for i := range expectedFingerprint {
		expectedFingerprint[i][1] *= 2
	}
	r := NewResolver(context.Background(), s.reader)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	r.AddSamples(1, s.indexed[1][0].Samples)
	resolved, err := r.Pprof(0)
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, pprofFingerprint(resolved, 0))
}

func Benchmark_block_Resolver_ResolvePprof_Small(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolvePprof(s.db, samples, 0))
	b.Run("1K", benchmarkResolverResolvePprof(s.db, samples, 1<<10))
	b.Run("8K", benchmarkResolverResolvePprof(s.db, samples, 8<<10))
}

func Benchmark_block_Resolver_ResolvePprof_Big(b *testing.B) {
	s := memSuite{t: b, files: [][]string{{"testdata/big-profile.pb.gz"}}}
	s.config = DefaultConfig().WithDirectory(b.TempDir())
	s.init()
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolvePprof(s.db, samples, 0))
	b.Run("8K", benchmarkResolverResolvePprof(s.db, samples, 8<<10))
	b.Run("16K", benchmarkResolverResolvePprof(s.db, samples, 16<<10))
	b.Run("32K", benchmarkResolverResolvePprof(s.db, samples, 32<<10))
	b.Run("64K", benchmarkResolverResolvePprof(s.db, samples, 64<<10))
}

func benchmarkResolverResolvePprof(sym SymbolsReader, samples v1.Samples, n int64) func(b *testing.B) {
	return func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := NewResolver(context.Background(), sym)
			r.AddSamples(0, samples)
			_, _ = r.Pprof(n)
		}
	}
}
