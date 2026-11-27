package block

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
	thanos "github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
	phlareparquet "github.com/grafana/pyroscope/v2/pkg/parquet"
)

type rangeCountingBucket struct {
	thanos.Bucket
	requests int
}

func (b *rangeCountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.requests++
	return b.Bucket.GetRange(ctx, name, off, length)
}

func TestExactParquetFooter(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	var data bytes.Buffer
	createParquetTestFile(t, &data, 10000)
	f, err := parquet.OpenFile(bytes.NewReader(data.Bytes()), int64(data.Len()))
	require.NoError(t, err)
	size, err := phlareparquet.FooterSize(f)
	require.NoError(t, err)
	require.Greater(t, size, uint64(footerSize(data.Bytes()))) // Includes both page indexes.
	payload := append(bytes.Repeat([]byte{0}, 23), data.Bytes()...)
	require.NoError(t, bucket.Upload(ctx, "profiles", bytes.NewReader(payload)))
	counter := &rangeCountingBucket{Bucket: bucket}
	ds := &Dataset{
		meta: &metastorev1.Dataset{ProfilesFooterSize: size, TableOfContents: []uint64{23, uint64(len(payload)), uint64(len(payload))}, Size: uint64(data.Len())},
		obj:  &Object{storage: objstore.NewBucket(counter), path: "profiles"},
	}
	require.NoError(t, openProfileTable(ctx, ds))
	p := ds.profiles
	defer p.Close()
	require.Equal(t, 1, counter.requests)
	validateColumnIndex(t, p.File, 10000)
	require.Equal(t, 1, counter.requests)
}
