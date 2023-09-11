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
	path   string
	size   int64
}

func (f *File) Open(ctx context.Context, b objstore.BucketReader, meta block.File, options ...parquet.FileOption) error {
	f.path = meta.RelPath
	f.size = int64(meta.SizeBytes)

	if f.size == 0 {
		attrs, err := b.Attributes(ctx, f.path)
		if err != nil {
			return fmt.Errorf("getting attributes: %w", err)
		}
		f.size = attrs.Size
	}
	var err error
	// the same reader is used to serve all requests, so we pass context.Background() here
	if f.reader, err = OptimizedBucketReaderAt(b, context.Background(), f.path); err != nil {
		return fmt.Errorf("creating reader: %w", err)
	}

	// first try to open file, this is required otherwise OpenFile panics
	f.File, err = parquet.OpenFile(f.reader, f.size,
		parquet.SkipPageIndex(true),
		parquet.SkipBloomFilters(true))
	if err != nil {
		return err
	}

	// now open it for real
	f.File, err = parquet.OpenFile(f.reader, f.size,
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

func (f *File) Path() string { return f.path }
