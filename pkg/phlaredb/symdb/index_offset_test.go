package symdb

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	pystore "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/stretchr/testify/require"
	thanos "github.com/thanos-io/objstore"
)

type indexCountingBucket struct {
	thanos.Bucket
	requests int
}

func (b *indexCountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.requests++
	return b.Bucket.GetRange(ctx, name, off, length)
}
func TestExactIndexOffset(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile("testdata/symbols/v3/" + DefaultFileName)
	require.NoError(t, err)
	offset, err := IndexOffset(data)
	require.NoError(t, err)
	for _, exact := range []bool{true, false} {
		name := "legacy"
		if exact {
			name = "exact"
		}
		t.Run(name, func(t *testing.T) {
			bucket := thanos.NewInMemBucket()
			require.NoError(t, bucket.Upload(ctx, "symbols", bytes.NewReader(data)))
			counter := &indexCountingBucket{Bucket: bucket}
			option := WithPrefetchSize(uint64(FooterSize)) // Force the legacy retry.
			expected := 2
			if exact {
				option = WithIndexOffset(offset)
				expected = 1
			}
			r, err := OpenObject(ctx, pystore.NewBucket(counter), "symbols", 0, int64(len(data)), option)
			require.NoError(t, err)
			require.NotNil(t, r.index)
			require.Equal(t, expected, counter.requests)
		})
	}
	_, err = OpenObject(ctx, pystore.NewBucket(thanos.NewInMemBucket()), "symbols", 0, int64(len(data)), WithIndexOffset(uint64(len(data))))
	require.Error(t, err)
	_, err = IndexOffset(nil)
	require.Error(t, err)
}
