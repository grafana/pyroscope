package arcticdb

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/thanos-io/objstore"
)

type BucketPrefixDecorator struct {
	objstore.Bucket
	prefix string
}

func (b *BucketPrefixDecorator) addPrefix(name string) string {
	return filepath.Join(b.prefix, name)
}

func (b *BucketPrefixDecorator) trimPrefix(name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, b.prefix), string(filepath.Separator))
}

func (b *BucketPrefixDecorator) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	iterFunc := func(path string) error {
		return f(b.trimPrefix(path))
	}
	return b.Bucket.Iter(ctx, b.addPrefix(dir), iterFunc, options...)
}

func (b *BucketPrefixDecorator) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.Bucket.Get(ctx, b.addPrefix(name))
}

func (b *BucketPrefixDecorator) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	return b.Bucket.GetRange(ctx, b.addPrefix(name), off, length)
}

func (b *BucketPrefixDecorator) Exists(ctx context.Context, name string) (bool, error) {
	return b.Bucket.Exists(ctx, b.addPrefix(name))
}

func (b *BucketPrefixDecorator) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	return b.Bucket.Attributes(ctx, b.addPrefix(name))
}

func (b *BucketPrefixDecorator) Upload(ctx context.Context, name string, r io.Reader) error {
	return b.Bucket.Upload(ctx, b.addPrefix(name), r)
}

func (b *BucketPrefixDecorator) Delete(ctx context.Context, name string) error {
	return b.Bucket.Delete(ctx, b.addPrefix(name))
}

func (b *BucketPrefixDecorator) Name() string {
	return b.addPrefix(b.Name())
}
