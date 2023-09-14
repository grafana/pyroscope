package filesystem

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/dskit/runutil"
	"github.com/pkg/errors"
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

func (b *Bucket) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	params := objstore.ApplyIterOptions(options...)
	if !params.WithoutAppendDirDelim || strings.HasSuffix(dir, objstore.DirDelim) {
		if dir != "" {
			dir = strings.TrimSuffix(dir, objstore.DirDelim) + objstore.DirDelim
		}
		return b.Bucket.Iter(ctx, dir, f, options...)
	}
	relDir := filepath.Dir(dir)
	prefix := dir
	return b.iterPrefix(ctx, filepath.Join(b.rootDir, relDir), relDir, prefix, f, options...)
}

// iterPrefix calls f for each entry in the given directory matching the prefix.
func (b *Bucket) iterPrefix(ctx context.Context, absDir string, relDir string, prefix string, f func(string) error, options ...objstore.IterOption) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	params := objstore.ApplyIterOptions(options...)
	info, err := os.Stat(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "stat %s", absDir)
	}
	if !info.IsDir() {
		return nil
	}

	files, err := os.ReadDir(absDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		name := filepath.Join(relDir, file.Name())
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}

		if file.IsDir() {
			empty, err := isDirEmpty(filepath.Join(absDir, file.Name()))
			if err != nil {
				return err
			}

			if empty {
				// Skip empty directories.
				continue
			}

			name += objstore.DirDelim

			if params.Recursive {
				// Recursively list files in the subdirectory.
				if err := b.iterPrefix(ctx, filepath.Join(absDir, file.Name()), name, prefix, f, options...); err != nil {
					return err
				}

				// The callback f() has already been called for the subdirectory
				// files so we should skip to next filesystem entry.
				continue
			}
		}
		if err := f(name); err != nil {
			return err
		}
	}
	return nil
}

func isDirEmpty(name string) (ok bool, err error) {
	f, err := os.Open(filepath.Clean(name))
	if os.IsNotExist(err) {
		// The directory doesn't exist. We don't consider it an error and we treat it like empty.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer runutil.CloseWithErrCapture(&err, f, "isDirEmpty")

	if _, err = f.Readdir(1); err == io.EOF || os.IsNotExist(err) {
		return true, nil
	}
	return false, err
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
