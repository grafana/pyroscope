package parquet

import (
	"context"

	phlareobjstore "github.com/grafana/pyroscope/pkg/objstore"
)

type optimizedReaderAt struct {
	phlareobjstore.ReaderAtCloser
	// todo: cache footer section we currently don't have a way to get the footer size from meta.
	// Not sure if we need to cache the footer size or not yet. Adding this to the footer size could help.
	// footerSize uint32
}

// NewOptimizedReader returns a reader that optimizes the reading of the parquet file.
func NewOptimizedReader(r phlareobjstore.ReaderAtCloser) phlareobjstore.ReaderAtCloser {
	return &optimizedReaderAt{
		ReaderAtCloser: r,
	}
}

// // called by parquet-go in OpenFile() to set offset and length of footer section
// func (r *optimizedReaderAt) SetFooterSection(offset, length int64) {
// 	// todo cache footer section
// }

// // called by parquet-go in OpenFile() to set offset and length of column indexes
// func (r *optimizedReaderAt) SetColumnIndexSection(offset, length int64) {
// 	// todo cache column index section
// }

// // called by parquet-go in OpenFile() to set offset and length of offset index section
// func (r *optimizedReaderAt) SetOffsetIndexSection(offset, length int64) {
// 	// todo cache offset index section
// }

func (r *optimizedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 4 && off == 0 {
		// Magic header
		return copy(p, []byte("PAR1")), nil
	}

	// // This requires knowing the footer size which we don't have access to in advance.
	// if len(p) == 8 && off == r.Size()-8 && r.footerSize > 0  {
	// 	// Magic footer
	// 	binary.LittleEndian.PutUint32(p, r.footerSize)
	// 	copy(p[4:8], []byte("PAR1"))
	// 	return 8, nil
	// }

	// todo handle cache
	return r.ReaderAtCloser.ReadAt(p, off)
}

// OptimizedBucketReaderAt uses a bucket reader and wraps the optimized reader. Must not be used with non-parquet files.
func OptimizedBucketReaderAt(bucketReader phlareobjstore.BucketReader, ctx context.Context, filename string) (phlareobjstore.ReaderAtCloser, error) {
	rc, err := bucketReader.ReaderAt(ctx, filename)
	if err != nil {
		return nil, err
	}
	return NewOptimizedReader(rc), nil
}
