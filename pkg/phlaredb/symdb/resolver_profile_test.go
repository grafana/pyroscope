package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_memory_Resolver_ResolveProfile(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Profile()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, profileFingerprint(resolved, 0))
}

func Test_block_Resolver_ResolveProfile_multiple_partitions(t *testing.T) {
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
	resolved, err := r.Profile()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, profileFingerprint(resolved, 0))
}

func Benchmark_block_Resolver_ResolveProfile(t *testing.B) {
	s := newBlockSuite(t, [][]string{{"testdata/big-profile.pb.gz"}})
	defer s.teardown()
	t.ResetTimer()
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		r := NewResolver(context.Background(), s.reader)
		r.AddSamples(0, s.indexed[0][0].Samples)
		_, _ = r.Profile()
	}
}
