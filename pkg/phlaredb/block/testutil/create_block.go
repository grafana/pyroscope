package testutil

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

type noLimit struct{}

func (n noLimit) AllowProfile(fp model.Fingerprint, lbs phlaremodel.Labels, tsNano int64) error {
	return nil
}

func (n noLimit) Stop() {}

// CreateBlock creates a block with the given profiles.
// Returns the block metadata, the directory where the block is stored, and an error if any.
func CreateBlock(t testing.TB, generator func() []*testhelper.ProfileBuilder) (block.Meta, string) {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	h, err := phlaredb.NewHead(ctx, phlaredb.Config{
		DataPath:         dir,
		MaxBlockDuration: 24 * time.Hour,
		Parquet: &phlaredb.ParquetConfig{
			MaxBufferRowCount: 10,
		},
	}, noLimit{})
	require.NoError(t, err)

	// ingest.
	for _, p := range generator() {
		require.NoError(t, h.Ingest(ctx, p.Profile, p.UUID, p.Labels...))
	}

	require.NoError(t, h.Flush(ctx))
	require.NoError(t, h.Move())
	localDir := filepath.Join(dir, phlaredb.PathLocal)
	metaMap, err := block.ListBlocks(localDir, time.Time{})
	require.NoError(t, err)
	require.Len(t, metaMap, 1)
	var meta *block.Meta
	for _, m := range metaMap {
		meta = m
	}
	require.NotNil(t, meta)
	return *meta, localDir
}

func OpenBlockFromMemory(t *testing.T, dir string, minTime, maxTime model.Time, profiles, tsdb, symbols []byte) *phlaredb.BlockQuerier {
	CreateBlockFromMemory(t, dir, minTime, maxTime, profiles, tsdb, symbols)
	blockBucket, err := filesystem.NewBucket(dir)
	require.NoError(t, err)
	blockQuerier := phlaredb.NewBlockQuerier(context.Background(), blockBucket)

	err = blockQuerier.Sync(context.Background())
	require.NoError(t, err)

	return blockQuerier
}

func CreateBlockFromMemory(t *testing.T, dir string, minTime, maxTime model.Time, profiles, tsdb, symbols []byte) *block.Meta {
	blockid, err := ulid.New(uint64(maxTime), rand.Reader)
	require.NoError(t, err)
	blockDir := filepath.Join(dir, blockid.String())
	err = os.MkdirAll(blockDir, 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(blockDir, "profiles.parquet"), profiles, 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(blockDir, "index.tsdb"), tsdb, 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(blockDir, "symbols.symdb"), symbols, 0644)
	assert.NoError(t, err)

	blockMeta := &block.Meta{
		ULID:    blockid,
		MinTime: minTime,
		MaxTime: maxTime,
		Files: []block.File{
			{
				RelPath:   "profiles.parquet",
				SizeBytes: uint64(len(profiles)),
			},
			{
				RelPath:   "index.tsdb",
				SizeBytes: uint64(len(tsdb)),
			},
			{
				RelPath:   "symbols.symdb",
				SizeBytes: uint64(len(symbols)),
			},
		},
		Version: block.MetaVersion3,
	}
	blockMetaJson, err := json.Marshal(&blockMeta)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(blockDir, block.MetaFilename), blockMetaJson, 0644)
	assert.NoError(t, err)

	return blockMeta
}
