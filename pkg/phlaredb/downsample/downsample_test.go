package downsample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	schemav1testhelper "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1/testhelper"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestDownsampler_ProfileCounts(t *testing.T) {
	outDir := t.TempDir()
	d, err := NewDownsampler(outDir, log.NewNopLogger())
	require.NoError(t, err)

	f, err := os.Open("../testdata/01HHYG6245NWHZWVP27V8WJRT7/profiles.parquet")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	reader := parquet.NewReader(f, schemav1.ProfilesSchema)
	rows, err := phlareparquet.ReadAllWithBufferSize(reader, 1024)
	require.NoError(t, err)

	for _, row := range rows {
		err = d.AddRow(schemav1.ProfileRow(row), 1)
		require.NoError(t, err)
	}

	err = d.Close()
	require.NoError(t, err)

	verifyProfileCount(t, outDir, "profiles_5m_sum.parquet", 1867)
	verifyProfileCount(t, outDir, "profiles_1h_sum.parquet", 1)
}

func TestDownsampler_Aggregation(t *testing.T) {
	profiles := make([]schemav1.InMemoryProfile, 0)
	builder := testhelper.NewProfileBuilder(1703853310000000000).CPUProfile() // 2023-12-29T12:35:10Z
	builder.ForStacktraceString("a", "b", "c").AddSamples(30)
	builder.ForStacktraceString("a", "b", "c", "d").AddSamples(20)
	batch, _ := schemav1testhelper.NewProfileSchema(builder.Profile, "cpu")
	profiles = append(profiles, batch...)

	builder = testhelper.NewProfileBuilder(1703853559000000000).CPUProfile() // 2023-12-29T12:39:19Z
	builder.ForStacktraceString("a", "b", "c").AddSamples(40)
	builder.ForStacktraceString("a", "b", "c", "d").AddSamples(30)
	builder.ForStacktraceString("a", "b", "c", "d", "e").AddSamples(20)
	batch, _ = schemav1testhelper.NewProfileSchema(builder.Profile, "cpu")
	profiles = append(profiles, batch...)

	builder = testhelper.NewProfileBuilder(1703854209000000000).CPUProfile() // 2023-12-29T12:50:09Z
	builder.ForStacktraceString("a", "b", "c").AddSamples(40)
	builder.ForStacktraceString("a", "b", "c", "d").AddSamples(30)
	batch, _ = schemav1testhelper.NewProfileSchema(builder.Profile, "cpu")
	profiles = append(profiles, batch...)

	builder = testhelper.NewProfileBuilder(1703858409000000000).CPUProfile() // 2023-12-29T14:00:09Z
	builder.ForStacktraceString("a", "b", "c").AddSamples(30)
	builder.ForStacktraceString("a", "b", "c", "d").AddSamples(20)
	batch, _ = schemav1testhelper.NewProfileSchema(builder.Profile, "cpu")
	profiles = append(profiles, batch...)

	reader := schemav1.NewInMemoryProfilesRowReader(profiles)
	rows, err := phlareparquet.ReadAllWithBufferSize(reader, 1024)
	require.NoError(t, err)

	outDir := t.TempDir()
	d, err := NewDownsampler(outDir, log.NewNopLogger())
	require.NoError(t, err)

	for _, row := range rows {
		err = d.AddRow(schemav1.ProfileRow(row), 1)
		require.NoError(t, err)
	}

	err = d.Close()
	require.NoError(t, err)

	downsampledRows := readDownsampledRows(t, filepath.Join(outDir, "profiles_5m_sum.parquet"), 3)

	schemav1.DownsampledProfileRow(downsampledRows[0]).ForValues(func(values []parquet.Value) {
		require.Equal(t, 3, len(values))
		require.Equal(t, int64(70), values[0].Int64()) // a, b, c
		require.Equal(t, int64(50), values[1].Int64()) // a, b, c, d
		require.Equal(t, int64(20), values[2].Int64()) // a, b, c, d, e
	})

	downsampledRows = readDownsampledRows(t, filepath.Join(outDir, "profiles_1h_sum.parquet"), 2)

	schemav1.DownsampledProfileRow(downsampledRows[0]).ForValues(func(values []parquet.Value) {
		require.Equal(t, 3, len(values))
		require.Equal(t, int64(110), values[0].Int64()) // a, b, c
		require.Equal(t, int64(80), values[1].Int64())  // a, b, c, d
		require.Equal(t, int64(20), values[2].Int64())  // a, b, c, d, e
	})
}

func TestDownsampler_VaryingFingerprints(t *testing.T) {
	profiles := make([]schemav1.InMemoryProfile, 0)
	for i := 0; i < 5; i++ {
		builder := testhelper.NewProfileBuilder(1703853310000000000).CPUProfile() // 2023-12-29T12:35:10Z
		builder.ForStacktraceString("a", "b", "c").AddSamples(30)
		batch, _ := schemav1testhelper.NewProfileSchema(builder.Profile, "cpu")
		profiles = append(profiles, batch...)
	}

	reader := schemav1.NewInMemoryProfilesRowReader(profiles)
	rows, err := phlareparquet.ReadAllWithBufferSize(reader, 1024)
	require.NoError(t, err)

	outDir := t.TempDir()
	d, err := NewDownsampler(outDir, log.NewNopLogger())
	require.NoError(t, err)

	for i, row := range rows {
		err = d.AddRow(schemav1.ProfileRow(row), model.Fingerprint(i))
		require.NoError(t, err)
	}

	err = d.Close()
	require.NoError(t, err)

	verifyProfileCount(t, outDir, "profiles_5m_sum.parquet", 5)
	verifyProfileCount(t, outDir, "profiles_1h_sum.parquet", 5)
}

func BenchmarkDownsampler_AddRow(b *testing.B) {
	f, err := os.Open("../testdata/01HHYG6245NWHZWVP27V8WJRT7/profiles.parquet")
	require.NoError(b, err)
	defer func() {
		require.NoError(b, f.Close())
	}()

	reader := parquet.NewGenericReader[*schemav1.Profile](f, schemav1.ProfilesSchema)
	rows, err := phlareparquet.ReadAllWithBufferSize(reader, 1024)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		outDir := b.TempDir()
		d, err := NewDownsampler(outDir, log.NewNopLogger())

		require.NoError(b, err)
		for _, row := range rows {
			err = d.AddRow(schemav1.ProfileRow(row), 1)
			require.NoError(b, err)
		}

		err = d.Close()
		require.NoError(b, err)
	}
}

func verifyProfileCount(t *testing.T, dir string, file string, expectedRows int) {
	stat, err := os.Stat(filepath.Join(dir, file))
	require.NoError(t, err)
	require.True(t, stat.Size() > 0)

	outFile, err := os.Open(filepath.Join(dir, file))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, outFile.Close())
	}()

	pf, err := parquet.OpenFile(outFile, stat.Size())
	require.NoError(t, err)

	outReader := parquet.NewReader(pf, schemav1.DownsampledProfilesSchema)
	require.Equal(t, int64(expectedRows), outReader.NumRows())
}

func readDownsampledRows(t *testing.T, path string, expectedRowCount int) []parquet.Row {
	stat, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, stat.Size() > 0)

	outFile, err := os.Open(path)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, outFile.Close())
	}()

	pf, err := parquet.OpenFile(outFile, stat.Size())
	require.NoError(t, err)

	reader := parquet.NewReader(pf, schemav1.DownsampledProfilesSchema)
	downsampledRows := make([]parquet.Row, 1000)
	rowCount, err := reader.ReadRows(downsampledRows)
	require.NoError(t, err)

	require.Equal(t, expectedRowCount, rowCount)
	return downsampledRows
}
