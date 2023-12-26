package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Load(t *testing.T) {
	s := newMemSuite(t, nil)
	s.init()
	s.writeProfileFromFile(0, "testdata/profile.pb.gz")
	s.db.PartitionWriter(1) // Empty partition.
	s.writeProfileFromFile(2, "testdata/profile.pb.gz")
	b := blockSuite{memSuite: s}
	b.flush()
	defer b.teardown()
	require.NoError(t, b.reader.Load(context.Background()))

	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	r := NewResolver(context.Background(), b.reader)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Pprof()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, pprofFingerprint(resolved, 0))

	expectedFingerprint = pprofFingerprint(s.profiles[2], 0)
	r = NewResolver(context.Background(), b.reader)
	defer r.Release()
	r.AddSamples(2, s.indexed[2][0].Samples)
	resolved, err = r.Pprof()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, pprofFingerprint(resolved, 0))
}
