package blocks

import (
	"context"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

func TestProfileHandlers_CloseDatasetOnce(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "../../../block/testdata")
	blockMeta, dataset := profileDataset(t)
	blockMeta.Size = math.MaxUint64 // Force each section to use the wrapped bucket reader.

	h := Handlers{Bucket: &singleCloseBucket{Bucket: bucket}}
	profiles, total, err := h.readProfilesFromDataset(ctx, blockMeta, dataset, 1, 10)
	require.NoError(t, err)
	require.NotZero(t, total)
	require.NotEmpty(t, profiles)

	profile, _, err := h.retrieveProfile(ctx, blockMeta, dataset, 0)
	require.NoError(t, err)
	require.NotNil(t, profile)
}

func profileDataset(t *testing.T) (*metastorev1.BlockMeta, *metastorev1.Dataset) {
	t.Helper()

	raw, err := os.ReadFile("../../../block/testdata/block-metas.json")
	require.NoError(t, err)
	var resp metastorev1.GetBlockMetadataResponse
	require.NoError(t, protojson.Unmarshal(raw, &resp))

	for _, blockMeta := range resp.Blocks {
		for _, dataset := range blockMeta.Datasets {
			if block.DatasetFormat(dataset.Format) == block.DatasetFormat0 {
				return blockMeta, dataset
			}
		}
	}
	t.Fatal("test data must contain a profile dataset")
	return nil, nil
}

type singleCloseBucket struct {
	objstore.Bucket
}

func (b *singleCloseBucket) ReaderAt(ctx context.Context, filename string) (objstore.ReaderAtCloser, error) {
	r, err := b.Bucket.ReaderAt(ctx, filename)
	if err != nil {
		return nil, err
	}
	return &singleCloseReader{ReaderAtCloser: r}, nil
}

type singleCloseReader struct {
	objstore.ReaderAtCloser
	mu     sync.Mutex
	closed bool
}

func (r *singleCloseReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		panic("reader closed more than once")
	}
	r.closed = true
	return r.ReaderAtCloser.Close()
}
