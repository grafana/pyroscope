package compactionworker

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	"github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func TestCompactBlocks(t *testing.T) {
	worker, err := New()
	require.NoError(t, err)

	ctx := context.Background()
	worker.storage, _ = testutil.NewFilesystemBucket(t, ctx, "testdata")

	var blockMetas compactorv1.BlockMetas
	blockMetasData, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(blockMetasData, &blockMetas)
	require.NoError(t, err)

	compactedBlocks, err := worker.compactBlocks(ctx, "job-123", blockMetas.Blocks)
	require.NoError(t, err)

	require.Len(t, compactedBlocks, 1)
}
