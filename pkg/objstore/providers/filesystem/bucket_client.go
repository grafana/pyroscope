package filesystem

import (
	"context"
	"os"
	"path/filepath"

	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/filesystem"

	phlareobjstore "github.com/grafana/pyroscope/pkg/objstore"
)

type Bucket struct {
	objstore.Bucket
	rootDir string
}

// NewBucket returns a new filesystem.Bucket.
func NewBucket(rootDir string, middlewares ...func(objstore.Bucket) (objstore.Bucket, error)) (*Bucket, error) {
	var (
		b   objstore.Bucket
		err error
	)
	b, err = filesystem.NewBucket(rootDir)
	if err != nil {
		return nil, err
	}
	for _, wrap := range middlewares {
		b, err = wrap(b)
		if err != nil {
			return nil, err
		}
	}
	return &Bucket{Bucket: b, rootDir: rootDir}, nil
}

func (b *Bucket) ReaderAt(ctx context.Context, filename string) (phlareobjstore.ReaderAtCloser, error) {
	f, err := os.Open(filepath.Join(b.rootDir, filename))
	if err != nil {
		return nil, err
	}

	return &FileReaderAt{File: f}, nil
}

// ReaderWithExpectedErrs implements objstore.Bucket.
func (b *Bucket) ReaderWithExpectedErrs(fn phlareobjstore.IsOpFailureExpectedFunc) phlareobjstore.BucketReader {
	return b.WithExpectedErrs(fn)
}

// WithExpectedErrs implements objstore.Bucket.
func (b *Bucket) WithExpectedErrs(fn phlareobjstore.IsOpFailureExpectedFunc) phlareobjstore.Bucket {
	if ib, ok := b.Bucket.(phlareobjstore.InstrumentedBucket); ok {
		return &Bucket{
			rootDir: b.rootDir,
			Bucket:  ib.WithExpectedErrs(fn),
		}
	}
	if ib, ok := b.Bucket.(objstore.InstrumentedBucket); ok {
		return &Bucket{
			rootDir: b.rootDir,
			Bucket:  ib.WithExpectedErrs(func(err error) bool { return fn(err) }),
		}
	}

	return b
}

type FileReaderAt struct {
	*os.File
}

func (b *FileReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	return b.File.ReadAt(p, off)
}
