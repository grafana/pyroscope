package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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

func Benchmark_block_Resolver_ResolveTree_Small(t *testing.B) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	t.ResetTimer()
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		r := NewResolver(context.Background(), s.db)
		r.AddSamples(0, s.indexed[0][0].Samples)
		_, _ = r.Tree()
	}
}

func Benchmark_block_Resolver_ResolveTree_Big(b *testing.B) {
	s := memSuite{t: b, files: [][]string{{"testdata/big-profile.pb.gz"}}}
	s.config = DefaultConfig().WithDirectory(b.TempDir())
	s.init()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := NewResolver(context.Background(), s.db)
		r.AddSamples(0, s.indexed[0][0].Samples)
		_, _ = r.Tree()
	}
}
