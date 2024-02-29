package pprof

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Benchmark_RepairGoTruncatedStacktraces(b *testing.B) {
	p, err := OpenFile("testdata/goheapfix/heap_go_truncated_3.pb.gz")
	require.NoError(b, err)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		RepairGoHeapTruncatedStacktraces(FixGoProfile(p.CloneVT()))
	}
}

func Test_UpdateFixtures_RepairGoTruncatedStacktraces(t *testing.T) {
	t.Skip()
	t.Helper()
	paths := []string{
		"testdata/goheapfix/heap_go_truncated_1.pb.gz", // Cortex.
		"testdata/goheapfix/heap_go_truncated_2.pb.gz", // Cortex.
		"testdata/goheapfix/heap_go_truncated_3.pb.gz", // Loki. Pathological.
		"testdata/goheapfix/heap_go_truncated_4.pb.gz", // Pyroscope.
	}
	for _, path := range paths {
		func() {
			p, err := OpenFile(path)
			require.NoError(t, err, path)
			f, err := os.Create(path + ".fixed")
			require.NoError(t, err, path)
			defer f.Close()
			p.Profile = FixGoProfile(p.Profile)
			RepairGoHeapTruncatedStacktraces(p.Profile)
			_, err = p.WriteTo(f)
			require.NoError(t, err, path)
		}()
	}
}
