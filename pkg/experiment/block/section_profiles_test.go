package block

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func createParquetFile[T any](t testing.TB, f io.Writer, rows []T, rowGroups int) {
	perRG := len(rows) / rowGroups

	w := parquet.NewGenericWriter[T](f)
	for i := 0; i < (rowGroups - 1); i++ {
		_, err := w.Write(rows[0:perRG])
		require.NoError(t, err)
		require.NoError(t, w.Flush())
		rows = rows[perRG:]
	}

	_, err := w.Write(rows)
	require.NoError(t, err)
	require.NoError(t, w.Flush())

	require.NoError(t, w.Close())
}

func createParquetTestFile(t testing.TB, f io.Writer, count int) {
	type T struct{ A int }

	rows := []T{}
	for i := 0; i < count; i++ {
		rows = append(rows, T{i})
	}

	createParquetFile(t, f, rows, 4)
}

func validateColumnIndex(t *testing.T, pf *parquet.File, count int) {
	rgs := pf.RowGroups()
	require.Equal(t, 4, len(rgs))

	// check last row groups column index
	ci, err := rgs[3].ColumnChunks()[0].ColumnIndex()
	require.NoError(t, err)

	pages := ci.NumPages()
	require.Equal(t, int64(count-1), ci.MaxValue(pages-1).Int64())
}

func Test_openParquetFile(t *testing.T) {
	path := "test.parquet"
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	buf := bytes.NewBuffer(nil)
	count := 100
	createParquetTestFile(t, buf, count)

	actualFooterSize := footerSize(buf.Bytes())

	err := bucket.Upload(ctx, path, bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	pathOffset := "test.offset.parquet"
	require.NoError(t, bucket.Upload(ctx, pathOffset, bytes.NewReader(append(bytes.Repeat([]byte{0xab}, 16), buf.Bytes()...))))

	opts := []parquet.FileOption{
		parquet.SkipBloomFilters(true),
	}

	t.Run("withFooterSizeSmallerThanEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), actualFooterSize*2, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})

	t.Run("withFooterSizeExactEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), actualFooterSize, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})
	t.Run("withFooterSizeSmallerEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), 200, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})
	t.Run("withFooterSizeVerySmall", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), 1, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})

	t.Run("withOffsetAndFooterSmallerThanEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, pathOffset, 16, int64(buf.Len()), actualFooterSize*2, opts...)
		require.NoError(t, err)
		validateColumnIndex(t, pf.File, count)
	})

}
