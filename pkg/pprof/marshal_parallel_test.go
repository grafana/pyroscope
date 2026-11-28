package pprof

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"strings"
	"testing"

	kgzip "github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

func TestMarshalParallelGzipCompatibilityAndReuse(t *testing.T) {
	data, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var source profilev1.Profile
	require.NoError(t, Unmarshal(data, &source))
	for _, size := range []int{0, 3 << 20, 100, 3 << 20, 0} {
		p := source.CloneVT()
		p.Comment = append(p.Comment, int64(len(p.StringTable)))
		p.StringTable = append(p.StringTable, strings.Repeat("profile-comment-", size/16))
		raw, err := p.MarshalVT()
		require.NoError(t, err)
		plain, err := Marshal(p, false)
		require.NoError(t, err)
		require.Equal(t, raw, plain)
		compressed, err := Marshal(p, true)
		require.NoError(t, err)
		if len(raw) < 1<<20 {
			var serial bytes.Buffer
			w := kgzip.NewWriter(&serial)
			_, err := w.Write(raw)
			require.NoError(t, err)
			require.NoError(t, w.Flush())
			require.NoError(t, w.Close())
			require.Equal(t, serial.Bytes(), compressed, "small profiles retain serial compression")
		}
		r, err := gzip.NewReader(bytes.NewReader(compressed))
		require.NoError(t, err)
		decoded, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		require.Equal(t, raw, decoded)
	}
}

// BenchmarkMarshalGzipFixtures measures the complete protobuf+gzip encoding
// path using only repository fixtures. Profile decoding is outside timing.
func BenchmarkMarshalGzipFixtures(b *testing.B) {
	for _, fixture := range []string{
		"go.cpu.labels.pprof",
		"dotnet.labels.pprof",
		"gotruncatefix/heap_go_truncated_3.pb.gz.fixed",
	} {
		b.Run(fixture, func(b *testing.B) {
			data, err := os.ReadFile("testdata/" + fixture)
			require.NoError(b, err)
			var p profilev1.Profile
			require.NoError(b, Unmarshal(data, &p))
			b.SetBytes(int64(p.SizeVT()))
			b.ReportAllocs()
			var encoded []byte
			for b.Loop() {
				encoded, err = Marshal(&p, true)
				if err != nil {
					b.Fatal(err)
				}
			}
			r, err := gzip.NewReader(bytes.NewReader(encoded))
			require.NoError(b, err)
			got, err := io.ReadAll(r)
			require.NoError(b, err)
			require.NoError(b, r.Close())
			want, err := p.MarshalVT()
			require.NoError(b, err)
			require.Equal(b, want, got)
		})
	}
}
