package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Load(t *testing.T) {
	s := newBlockResolverSuite(t, [][]string{
		{"testdata/profile.pb.gz"},
		{"testdata/profile.pb.gz"},
	})
	defer s.teardown()
	require.NoError(t, s.reader.Load(context.Background()))

	expectedFingerprint := pprofFingerprint(s.profiles[0].Profile, 0)
	r := NewResolver(context.Background(), s.reader)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Profile()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, profileFingerprint(resolved, 0))

	expectedFingerprint = pprofFingerprint(s.profiles[1].Profile, 0)
	r = NewResolver(context.Background(), s.reader)
	defer r.Release()
	r.AddSamples(1, s.indexed[1][0].Samples)
	resolved, err = r.Profile()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, profileFingerprint(resolved, 0))
}
