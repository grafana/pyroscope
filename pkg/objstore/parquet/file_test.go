package parquet

import (
	"context"
	"os"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type readerAtCall struct {
	offset int64
	size   int64
}

type readerAtLogger struct {
	objstore.ReaderAtCloser
	calls []readerAtCall
}

func (r *readerAtLogger) ReadAt(p []byte, off int64) (n int, err error) {
	r.calls = append(r.calls, readerAtCall{offset: off, size: int64(len(p))})
	return r.ReaderAtCloser.ReadAt(p, off)

}

type bucketReadRangeLogger struct {
	objstore.BucketReader
	lastReaderAt *readerAtLogger
}

func (b *bucketReadRangeLogger) ReaderAt(ctx context.Context, filename string) (objstore.ReaderAtCloser, error) {
	readerAt, err := b.BucketReader.ReaderAt(ctx, filename)
	b.lastReaderAt = &readerAtLogger{
		ReaderAtCloser: readerAt,
	}
	return b.lastReaderAt, err
}

func newBucketReader(t *testing.T, path string) *bucketReadRangeLogger {
	bucketClient, err := filesystem.NewBucket(path)
	require.NoError(t, err)

	return &bucketReadRangeLogger{BucketReader: objstore.NewBucket(bucketClient)}
}

func newParquetFile(t *testing.T, rowCount int) (block.File, *bucketReadRangeLogger) {
	batch := 10

	type Row struct{ N, NTime2, NTimes3 int }

	rows := make([]Row, batch)
	pos := 0

	tempDir := t.TempDir()
	fileName := "test.parquet"

	output, err := os.Create(tempDir + "/" + fileName)
	require.NoError(t, err)

	writer := parquet.NewGenericWriter[Row](output)

	for {
		for idx := range rows {
			rows[idx].N = pos
			rows[idx].NTime2 = pos * 2
			rows[idx].NTimes3 = pos * 3
			pos += 1

			if pos >= rowCount {
				rows = rows[:idx+1]
				break
			}
		}

		_, err = writer.Write(rows)
		require.NoError(t, err)

		if pos >= rowCount {
			break
		}
	}

	// closing the writer is necessary to flush buffers and write the file footer.
	require.NoError(t, writer.Close())

	// get file size
	fi, err := output.Stat()
	require.NoError(t, err)

	return block.File{
		RelPath:   "test.parquet",
		SizeBytes: uint64(fi.Size()),
		Parquet:   &block.ParquetFile{},
	}, newBucketReader(t, tempDir)
}

const (
	parquetReadBufferSize = 256 << 10 // 256KB
)

func DefaultFileOptions() []parquet.FileOption {
	return []parquet.FileOption{
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}
}

func TestFile_Open(t *testing.T) {
	var f File

	t.Run("small parquet file, ensure single request to bucket", func(t *testing.T) {
		meta, bucketReader := newParquetFile(t, 100)

		require.NoError(t, f.Open(context.Background(), bucketReader, meta, DefaultFileOptions()...))
		require.Len(t, bucketReader.lastReaderAt.calls, 1)

		// parquet file smalle, so cache will actually hold all of it
		assert.Equal(t, int64(0), bucketReader.lastReaderAt.calls[0].offset)
		assert.Equal(t, int64(meta.SizeBytes), bucketReader.lastReaderAt.calls[0].size)
	})

	t.Run("bigger parquet file, ensure single request to bucket", func(t *testing.T) {
		meta, bucketReader := newParquetFile(t, 100_000)

		require.NoError(t, f.Open(context.Background(), bucketReader, meta, DefaultFileOptions()...))
		require.Len(t, bucketReader.lastReaderAt.calls, 1)

		// parquet file will use the minimum 32KiB cache size
		assert.Equal(t, int64(meta.SizeBytes-(32*1024)), bucketReader.lastReaderAt.calls[0].offset)
		assert.Equal(t, int64(32*1024), bucketReader.lastReaderAt.calls[0].size)
	})
}
