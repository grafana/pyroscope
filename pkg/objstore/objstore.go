package objstore

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/thanos-io/objstore"
)

type ReaderAt interface {
	io.ReaderAt
	io.Closer
	Size() int64
}

type Bucket interface {
	objstore.Bucket
	ReaderAt(ctx context.Context, filename string) (ReaderAt, error)
}

type BucketReader interface {
	objstore.BucketReader
	ReaderAtCreator
}

type ReaderAtCreator interface {
	ReaderAt(ctx context.Context, filename string) (ReaderAt, error)
}

func BucketReaderWithPrefix(r BucketReader, prefix string) BucketReader {
	if !strings.HasSuffix(prefix, objstore.DirDelim) {
		prefix += objstore.DirDelim
	}

	return &bucketReaderWithPrefix{
		r: r,
		p: prefix,
	}
}

type bucketReaderWithPrefix struct {
	r BucketReader
	p string
}

func (b *bucketReaderWithPrefix) prefix(path string) string {
	return b.p + path
}

// Iter calls f for each entry in the given directory (not recursive.). The argument to f is the full
// object name including the prefix of the inspected directory.
// Entries are passed to function in sorted order.
func (b *bucketReaderWithPrefix) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	return b.r.Iter(ctx, b.prefix(dir), func(s string) error {
		return f(strings.TrimPrefix(s, b.p))
	})
}

// Get returns a reader for the given object name.
func (b *bucketReaderWithPrefix) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.r.Get(ctx, b.prefix(name))
}

// GetRange returns a new range reader for the given object name and range.
func (b *bucketReaderWithPrefix) GetRange(ctx context.Context, name string, off int64, length int64) (io.ReadCloser, error) {
	return b.r.GetRange(ctx, b.prefix(name), off, length)
}

// Exists checks if the given object exists in the bucket.
func (b *bucketReaderWithPrefix) Exists(ctx context.Context, name string) (bool, error) {
	return b.r.Exists(ctx, b.prefix(name))
}

// IsObjNotFoundErr returns true if error means that object is not found. Relevant to Get operations.
func (b *bucketReaderWithPrefix) IsObjNotFoundErr(err error) bool {
	return b.r.IsObjNotFoundErr(err)
}

// Attributes returns information about the specified object.
func (b *bucketReaderWithPrefix) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	return b.r.Attributes(ctx, b.prefix(name))
}

func (b *bucketReaderWithPrefix) ReaderAt(ctx context.Context, name string) (ReaderAt, error) {
	return b.r.ReaderAt(ctx, b.prefix(name))
}

type bucketWithPrefix struct {
	BucketReader

	b Bucket
	p string
}

func BucketWithPrefix(b Bucket, prefix string) Bucket {
	if !strings.HasSuffix(prefix, objstore.DirDelim) {
		prefix += objstore.DirDelim
	}

	return &bucketWithPrefix{
		BucketReader: BucketReaderWithPrefix(b, prefix),
		b:            b,
		p:            prefix,
	}
}

func (b *bucketWithPrefix) Close() error {
	return b.b.Close()
}

func (b *bucketWithPrefix) Upload(ctx context.Context, name string, r io.Reader) error {
	return b.b.Upload(ctx, b.prefix(name), r)
}

func (b *bucketWithPrefix) Delete(ctx context.Context, name string) error {
	return b.b.Delete(ctx, b.prefix(name))
}

func (b *bucketWithPrefix) Name() string {
	return filepath.Join(b.b.Name(), b.p)
}

func (b *bucketWithPrefix) prefix(path string) string {
	return b.p + path
}
