package convert

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkPprofToProfile(b *testing.B) {
	f, err := os.Open("./testdata/cpu-unknown.pb.gz")
	require.NoError(b, err)
	defer f.Close()
	data, err := io.ReadAll(f)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PprofToProfile(data, "test", 16384)
	}
	b.StopTimer()
}
