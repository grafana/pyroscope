package main

import (
	"bytes"
	"context"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

func TestDumpBlock_MultipleDatasets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "../../pkg/block/testdata")

	raw, err := os.ReadFile("../../pkg/block/testdata/block-metas.json")
	require.NoError(t, err)
	var resp metastorev1.GetBlockMetadataResponse
	require.NoError(t, protojson.Unmarshal(raw, &resp))

	var md *metastorev1.BlockMeta
	for _, b := range resp.Blocks {
		if len(b.Datasets) > 1 {
			md = b
			break
		}
	}
	require.NotNil(t, md, "test data must contain a block with more than one dataset")

	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, replayHeader{})
	require.NoError(t, err)

	count, err := dumpBlock(ctx, bucket, md, nil, 0, math.MaxInt64, rw)
	require.NoError(t, err)
	assert.NotZero(t, count)
	require.NoError(t, rw.Flush())
}
