package firedb

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

type pointerSlice struct {
	Values []*value
}

type value struct {
	World string
}

func TestReproduce(t *testing.T) {
	p := &pointerSlice{
		Values: []*value{
			{World: "Helloe"},
		},
	}

	sch := parquet.SchemaOf(p)
	t.Logf("%v", sch.Columns())

	buffer := new(bytes.Buffer)
	pw := parquet.NewWriter(buffer, sch)

	require.NoError(t, pw.Write(p))
}

func parseProfile(t testing.TB, path string) *profilev1.Profile {

	f, err := os.Open(path)
	require.NoError(t, err, "failed opening profile: ", path)
	r, err := gzip.NewReader(f)
	require.NoError(t, err)
	content, err := ioutil.ReadAll(r)
	require.NoError(t, err, "failed reading file: ", path)

	p := &profilev1.Profile{}
	require.NoError(t, p.UnmarshalVT(content))

	return p
}

// This verifies that
func TestRoundTrip(t *testing.T) {
	var (
		profilePaths = []string{
			"testdata/heap",
			"testdata/profile",
		}
		profiles = make([]*profilev1.Profile, len(profilePaths))
	)
	for pos := range profilePaths {
		profiles[pos] = parseProfile(t, profilePaths[pos])
	}

	buffer := new(bytes.Buffer)

	t.Run("ingest", func(t *testing.T) {
		sch := parquet.SchemaOf(&profilev1.Profile{})
		pw := parquet.NewWriter(buffer, sch)

		for pos := range profiles {
			require.NoError(t, pw.Write(profiles[pos]), "error writing profile ", profilePaths[pos])
		}

		require.NoError(t, pw.Close())
	})

	t.Run("read-verify", func(t *testing.T) {
		rows, err := parquet.Read[*profilev1.Profile](bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
		require.Error(t, io.EOF, err)
		require.Equal(t, len(profiles), len(rows))

		for pos := range rows {
			// ensure empty slice becomes nil slice, otherwise the equal fails
			for _, s := range rows[pos].Sample {
				if len(s.Label) == 0 {
					s.Label = nil
				}
			}
			require.Equal(t, profiles[pos].Sample, rows[pos].Sample)

			// test other fields exported
			require.Equal(t, profiles[pos].SampleType, rows[pos].SampleType)
			require.Equal(t, profiles[pos].Mapping, rows[pos].Mapping)
			require.Equal(t, profiles[pos].Location, rows[pos].Location)
			require.Equal(t, profiles[pos].Function, rows[pos].Function)
			require.Equal(t, profiles[pos].TimeNanos, rows[pos].TimeNanos)
			require.Equal(t, profiles[pos].DurationNanos, rows[pos].DurationNanos)
			require.Equal(t, profiles[pos].PeriodType, rows[pos].PeriodType)
		}

	})

}
