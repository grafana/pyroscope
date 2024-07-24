package block

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	"github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func Test_CompactBlocks(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "testdata")

	var blockMetas compactorv1.CompletedJob // same contract, can break in the future
	blockMetasData, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(blockMetasData, &blockMetas)
	require.NoError(t, err)

	dst, tempdir := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	compactedBlocks, err := Compact(ctx, blockMetas.Blocks, bucket,
		WithCompactionDestination(dst),
		WithCompactionObjectOptions(
			WithObjectDownload(filepath.Join(tempdir, "source")),
			WithObjectMaxSizeLoadInMemory(0)), // Force download.
	)

	require.NoError(t, err)
	require.Len(t, compactedBlocks, 1)
	// TODO: Assertions.
}
