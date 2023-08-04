package symdb

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/pprof"
)

func Test_symdb_read_write(t *testing.T) {
	cfg := &Config{
		Dir: t.TempDir(),
		Stacktraces: StacktracesConfig{
			MaxNodesPerChunk: 1 << 20,
		},
		Parquet: ParquetConfig{
			MaxBufferRowCount: 100 << 10,
		},
	}

	db := NewSymDB(cfg)
	w := db.SymbolsWriter(1)
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	m1 := w.WriteProfileSymbols(p.Profile)

	w = db.SymbolsWriter(2)
	p, err = pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	_ = w.WriteProfileSymbols(p.Profile)

	require.NoError(t, db.Flush())

	b, err := filesystem.NewBucket(cfg.Dir)
	require.NoError(t, err)

	r, err := Open(context.Background(), b)
	require.NoError(t, err)

	p1, err := r.SymbolsReader(context.Background(), 1)
	require.NoError(t, err)

	sr := Resolver{
		Stacktraces: p1,
		Locations:   p1.locations.slice,
		Mappings:    p1.mappings.slice,
		Functions:   p1.functions.slice,
		Strings:     p1.strings.slice,
	}

	resolved, err := sr.ResolveProfile(context.Background(), m1[0].Samples)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = resolved.WriteUncompressed(&buf)
	require.NoError(t, err)

	p, err = pprof.RawFromBytes(buf.Bytes())
	require.NoError(t, err)
}
