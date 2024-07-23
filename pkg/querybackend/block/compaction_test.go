package block

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
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

	compactedBlocks, err := Compact(ctx, blockMetas.Blocks, bucket)
	require.NoError(t, err)

	// TODO: Assertions.
	require.Len(t, compactedBlocks, 1)
	metas := []*metastorev1.BlockMeta{
		compactedBlocks[0],
		compactedBlocks[0].CloneVT(),
	}

	compactedBlocks, err = Compact(ctx, metas, bucket)
	require.NoError(t, err)
}
