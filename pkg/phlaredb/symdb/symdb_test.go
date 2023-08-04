package symdb

import (
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
	w.WriteProfileSymbols(p.Profile)

	w = db.SymbolsWriter(2)
	p, err = pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	w.WriteProfileSymbols(p.Profile)

	require.NoError(t, db.Flush())

	b, err := filesystem.NewBucket(cfg.Dir)
	require.NoError(t, err)

	r, err := Open(context.Background(), b)
	require.NoError(t, err)

	p1, err := r.SymbolsReader(context.Background(), 1)
	require.NoError(t, err)

	_ = p1
	/*
		locs, err := p1.Locations(context.Background(), nil)
		require.NoError(t, err)
		require.NoError(t, locs.Close())

		maps, err := p1.Mappings(context.Background(), nil)
		require.NoError(t, err)
		require.NoError(t, maps.Close())

		funcs, err := p1.Functions(context.Background(), nil)
		require.NoError(t, err)
		require.NoError(t, funcs.Close())

		strings, err := p1.Strings(context.Background(), nil)
		require.NoError(t, err)
		require.NoError(t, strings.Close())*/
}
