package parquet

import (
	"context"
	"fmt"

	"github.com/parquet-go/parquet-go"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type File struct {
	*parquet.File
	reader objstore.ReaderAtCloser
	meta   block.File
}

func (f *File) Open(ctx context.Context, b objstore.BucketReader, meta block.File, options ...parquet.FileOption) error {
	if meta.SizeBytes == 0 {
		attrs, err := b.Attributes(ctx, meta.RelPath)
		if err != nil {
			return fmt.Errorf("getting attributes: %w", err)
		}
		meta.SizeBytes = uint64(attrs.Size)
	}
	var err error
	// the same reader is used to serve all requests, so we pass context.Background() here
	ra, err := OptimizedBucketReaderAt(b, context.Background(), meta)
	if err != nil {
		return fmt.Errorf("creating reader: %w", err)
	}
	f.reader = ra
	ora := ra.(*optimizedReaderAt)

	// after finishing opening, clear footer cache
	defer ora.clearFooterCache()

	// first try to open file, this is required otherwise OpenFile panics
	f.File, err = parquet.OpenFile(f.reader, int64(meta.SizeBytes),
		parquet.SkipPageIndex(true),
		parquet.SkipBloomFilters(true))
	if err != nil {
		return err
	}

	// now open it for real
	f.File, err = parquet.OpenFile(f.reader, int64(meta.SizeBytes),
		options...,
	)
	return err
}

func (f *File) Close() (err error) {
	if f.reader != nil {
		return f.reader.Close()
	}
	return nil
}

func (f *File) Path() string { return f.meta.RelPath }
