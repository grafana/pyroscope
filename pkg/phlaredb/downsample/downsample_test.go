package downsample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"

	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func TestDownsampler_AddRow(t *testing.T) {
	outDir := t.TempDir()
	d, err := NewDownsampler(outDir)
	require.NoError(t, err)

	f, err := os.Open("../testdata/01HHYG6245NWHZWVP27V8WJRT7/profiles.parquet")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	reader := parquet.NewGenericReader[*schemav1.Profile](f, schemav1.ProfilesSchema)
	rows, err := phlareparquet.ReadAllWithBufferSize(reader, 1024)
	require.NoError(t, err)

	for _, row := range rows {
		err = d.AddRow(schemav1.ProfileRow(row), 1)
		require.NoError(t, err)
	}

	err = d.Close()
	require.NoError(t, err)

	verifyOutput(t, outDir, "profiles_5m_sum.parquet", 1867)
	verifyOutput(t, outDir, "profiles_1h_sum.parquet", 1)
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
		d, err := NewDownsampler(outDir)

		require.NoError(b, err)
		for _, row := range rows {
			err = d.AddRow(schemav1.ProfileRow(row), 1)
			require.NoError(b, err)
		}

		err = d.Close()
		require.NoError(b, err)
	}
}

func verifyOutput(t *testing.T, dir string, file string, expectedRows int) {
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
