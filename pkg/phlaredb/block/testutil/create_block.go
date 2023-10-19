package testutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
