package block_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	thanosstore "github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

type rangeCountingBucket struct {
	objstore.Bucket
	getRange int
}

func (b *rangeCountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.getRange++
	return b.Bucket.GetRange(ctx, name, off, length)
}

func TestNewObjectFromPath(t *testing.T) {
	tests := []struct {
		name          string
		dataSize      int
		metaString    int
		expectedReads int
	}{
		{name: "object smaller than the footer read", dataSize: 64, metaString: 16, expectedReads: 1},
		{name: "metadata within the footer read", dataSize: 4 << 20, metaString: 32 << 10, expectedReads: 1},
		{name: "metadata larger than the footer read", dataSize: 4 << 20, metaString: 1 << 20, expectedReads: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := metadata.NewStringTable()
			md := &metastorev1.BlockMeta{
				FormatVersion:   1,
				Id:              "01J000000000000000000000A1",
				Shard:           1,
				CompactionLevel: 1,
				Tenant:          st.Put("tenant-a"),
				CreatedBy:       st.Put(strings.Repeat("c", tt.metaString)),
				MinTime:         1,
				MaxTime:         2,
			}
			md.StringTable = st.Strings

			var body bytes.Buffer
			body.WriteString(strings.Repeat("x", tt.dataSize))
			md.MetadataOffset = uint64(body.Len())
			require.NoError(t, metadata.Encode(&body, md))

			bucket := &rangeCountingBucket{Bucket: objstore.NewBucket(thanosstore.NewInMemBucket())}
			path := block.ObjectPath(md)
			require.NoError(t, bucket.Upload(ctx, path, bytes.NewReader(body.Bytes())))

			obj, err := block.NewObjectFromPath(ctx, bucket, path)
			require.NoError(t, err)
			require.Equal(t, tt.expectedReads, bucket.getRange)

			got := obj.Metadata()
			require.Equal(t, md.Id, got.Id)
			require.Equal(t, md.MetadataOffset, got.MetadataOffset)
			require.Equal(t, md.StringTable, got.StringTable)
			require.Equal(t, uint64(body.Len()), got.Size)
		})
	}
}
