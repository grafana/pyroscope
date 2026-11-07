package pprof

import (
	"os"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
)

func TestOwnedDirectDetachesRetainedStrings(t *testing.T) {
	encoded, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var source profilev1.Profile
	require.NoError(t, Unmarshal(encoded, &source))
	originals := make(map[string]*byte)
	for _, s := range source.StringTable {
		if s != "" {
			originals[s] = unsafe.StringData(s)
		}
	}
	var expected, actual ProfileMerge
	require.NoError(t, expected.MergeBytes(encoded, true))
	require.NoError(t, actual.MergeOwned(&source, true))
	testhelper.EqualProto(t, expected.Profile(), actual.Profile())
	for _, s := range actual.Profile().StringTable {
		if s != "" {
			require.False(t, originals[s] == unsafe.StringData(s), "retained string aliases input storage: %q", s)
		}
	}
}
