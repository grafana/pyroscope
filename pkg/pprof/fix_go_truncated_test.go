package pprof

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Benchmark_RepairGoTruncatedStacktraces(b *testing.B) {
	p, err := OpenFile("testdata/gotruncatefix/heap_go_truncated_3.pb.gz")
	require.NoError(b, err)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		RepairGoTruncatedStacktraces(FixGoProfile(p.CloneVT()))
	}
}

func Test_UpdateFixtures_RepairGoTruncatedStacktraces(t *testing.T) {
	if os.Getenv("UPDATE_FIXTURES") != "true" {
		t.Skip()
	}
	t.Helper()
	paths := []string{
		"testdata/gotruncatefix/heap_go_truncated_1.pb.gz", // Cortex.
		"testdata/gotruncatefix/heap_go_truncated_2.pb.gz", // Cortex.
		"testdata/gotruncatefix/heap_go_truncated_3.pb.gz", // Loki. Pathological.
		"testdata/gotruncatefix/heap_go_truncated_4.pb.gz", // Pyroscope.
		"testdata/gotruncatefix/cpu_go_truncated_1.pb.gz",  // Cloudwatch Exporter
	}
	for _, path := range paths {
		func() {
			p, err := OpenFile(path)
			require.NoError(t, err, path)
			f, err := os.Create(path + ".fixed")
			require.NoError(t, err, path)
			defer f.Close()
			p.Profile = FixGoProfile(p.Profile)
			RepairGoTruncatedStacktraces(p.Profile)
			_, err = p.WriteTo(f)
			require.NoError(t, err, path)
		}()
	}
}
