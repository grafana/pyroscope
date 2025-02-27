package block

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func Test_CompactBlocks(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "testdata")

	var resp metastorev1.GetBlockMetadataResponse
	raw, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(raw, &resp)
	require.NoError(t, err)

	dst, tempdir := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	compactedBlocks, err := Compact(ctx, resp.Blocks, bucket,
		WithCompactionDestination(dst),
		WithCompactionTempDir(tempdir),
		WithCompactionObjectOptions(
			WithObjectDownload(filepath.Join(tempdir, "source")),
			WithObjectMaxSizeLoadInMemory(0)), // Force download.
	)

	require.NoError(t, err)
	require.Len(t, compactedBlocks, 1)
	require.NotZero(t, compactedBlocks[0].Size)
	require.Len(t, compactedBlocks[0].Datasets, 4)

	compactedJson, err := json.MarshalIndent(compactedBlocks, "", "  ")
	require.NoError(t, err)
	expectedJson, err := os.ReadFile("testdata/compacted.golden")
	require.NoError(t, err)
	assert.Equal(t, string(expectedJson), string(compactedJson))

	t.Run("Compact compacted blocks", func(t *testing.T) {
		compactedBlocks, err = Compact(ctx, compactedBlocks, dst,
			WithCompactionDestination(dst),
			WithCompactionTempDir(tempdir),
			WithCompactionObjectOptions(
				WithObjectDownload(filepath.Join(tempdir, "source")),
				WithObjectMaxSizeLoadInMemory(0)), // Force download.
		)

		require.NoError(t, err)
		require.Len(t, compactedBlocks, 1)
		require.NotZero(t, compactedBlocks[0].Size)
		require.Len(t, compactedBlocks[0].Datasets, 4)
	})
}
