package compactionworker

import (
	"context"
	"os"
	"testing"

	"github.com/grafana/pyroscope/ebpf/util"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	"github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func TestCompactBlocks(t *testing.T) {
	worker, err := New(util.TestLogger(t), nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	worker.storage, _ = testutil.NewFilesystemBucket(t, ctx, "testdata")

	var blockMetas compactorv1.CompletedJob // same contract, can break in the future
	blockMetasData, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(blockMetasData, &blockMetas)
	require.NoError(t, err)

	compactedBlocks, err := worker.compactBlocks(ctx, "job-123", blockMetas.Blocks)
	require.NoError(t, err)

	require.Len(t, compactedBlocks, 1)

	_ = worker.storage.Delete(ctx, "blocks")
}
