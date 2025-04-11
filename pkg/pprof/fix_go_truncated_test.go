package pprof

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
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

func generateStackTrace(n int) []string {
	res := make([]string, n)
	runes := []rune("abcdefghijklmnopqrstuvwxyz")

	for idx := range res {
		dest := n - (idx + 1)
		if idx == 0 {
			res[dest] = "start"
			continue
		}
		res[dest] = fmt.Sprintf("%c%d", runes[(idx-1)/10], (idx-1)%10)
	}
	return res
}

func Test_RepairGoTruncatedStacktraces(t *testing.T) {
	n := 128
	fullStack := generateStackTrace(n)
	b := testhelper.NewProfileBuilder(0).CPUProfile()
	b.ForStacktraceString(fullStack[n-24:]...).AddSamples(1)
	b.ForStacktraceString(fullStack[n-58 : n-9]...).AddSamples(2)
	b.ForStacktraceString(fullStack[n-57 : n-8]...).AddSamples(4)
	b.ForStacktraceString(fullStack[n-56 : n-7]...).AddSamples(8)
	b.ForStacktraceString(append([]string{"yy1"}, fullStack[n-22:]...)...).AddSamples(16)

	RepairGoTruncatedStacktraces(b.Profile)

	// ensure all stacktraces start with the same 8 location ids
	stacks := make([]uint64, 8)
	for idx, sample := range b.Profile.Sample {
		first8Stacks := sample.LocationId[len(sample.LocationId)-8:]
		if idx == 0 {
			copy(stacks, first8Stacks)
			continue
		}
		t.Log(stacks)
		assert.Equal(t, stacks, first8Stacks)
	}
}

var goTruncatedStacktracesFixtures = []string{
	"testdata/gotruncatefix/heap_go_truncated_1.pb.gz", // Cortex.
	"testdata/gotruncatefix/heap_go_truncated_2.pb.gz", // Cortex.
	"testdata/gotruncatefix/heap_go_truncated_3.pb.gz", // Loki. Pathological.
	"testdata/gotruncatefix/heap_go_truncated_4.pb.gz", // Pyroscope.
	"testdata/gotruncatefix/cpu_go_truncated_1.pb.gz",  // Cloudwatch Exporter
}

func Test_RepairGoTruncatedStacktraces_Fixtures(t *testing.T) {
	for _, path := range goTruncatedStacktracesFixtures {
		p, err := OpenFile(path)
		require.NoError(t, err, path)
		total := samplesTotal(p.Profile)

		p.Profile = FixGoProfile(p.Profile)
		assert.Equal(t, total, samplesTotal(p.Profile))

		p.Normalize()
		assert.Equal(t, total, samplesTotal(p.Profile))

		fixed, err := OpenFile(path + ".fixed")
		require.NoError(t, err)
		assert.Equal(t, total, samplesTotal(fixed.Profile))
	}
}

func Test_UpdateFixtures_RepairGoTruncatedStacktraces(t *testing.T) {
	if os.Getenv("UPDATE_FIXTURES") != "true" {
		t.Skip()
	}
	for _, path := range goTruncatedStacktracesFixtures {
		p, err := OpenFile(path)
		require.NoError(t, err, path)
		total := samplesTotal(p.Profile)

		p.Profile = FixGoProfile(p.Profile)
		p.Normalize()
		assert.Equal(t, total, samplesTotal(p.Profile))

		path += ".fixed"
		fixed, err := os.Create(path)
		require.NoError(t, err, path)
		_, err = p.WriteTo(fixed)
		require.NoError(t, fixed.Close(), path)
		require.NoError(t, err, path)
	}
}
