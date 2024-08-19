package objstore

import (
	"context"
	"fmt"
	"io"

	"github.com/thanos-io/objstore"
)

type ReaderAtCloser interface {
	io.ReaderAt
	io.Closer
}

func NewBucket(bkt objstore.Bucket) Bucket {
	if bucket, ok := bkt.(Bucket); ok {
		return bucket
	}
	return &ReaderAtBucket{
		Bucket: bkt,
	}
}

type ReaderAtBucket struct {
	objstore.Bucket
}

func (b *ReaderAtBucket) ReaderAt(ctx context.Context, name string) (ReaderAtCloser, error) {
	return &ReaderAt{
		GetRangeReader: b.Bucket,
		name:           name,
		ctx:            ctx,
	}, nil
}

// ReaderWithExpectedErrs implements objstore.Bucket.
func (b *ReaderAtBucket) ReaderWithExpectedErrs(fn IsOpFailureExpectedFunc) BucketReader {
	return b.WithExpectedErrs(fn)
}

// WithExpectedErrs implements objstore.Bucket.
func (b *ReaderAtBucket) WithExpectedErrs(fn IsOpFailureExpectedFunc) Bucket {
	if ib, ok := b.Bucket.(InstrumentedBucket); ok {
		return &ReaderAtBucket{
			Bucket: ib.WithExpectedErrs(fn),
		}
	}

	if ib, ok := b.Bucket.(objstore.InstrumentedBucket); ok {
		return &ReaderAtBucket{
			Bucket: ib.WithExpectedErrs(func(err error) bool { return fn(err) }),
		}
	}

	return b
}

type GetRangeReader interface {
	GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error)
}

type ReaderAt struct {
	GetRangeReader
	name string
	ctx  context.Context
}

func (b *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	rc, err := b.GetRangeReader.GetRange(b.ctx, b.name, off, int64(len(p)))
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	totalBytes := 0
	for {
		byteCount, err := rc.Read(p[totalBytes:])
		totalBytes += byteCount
		if err == io.EOF {
			return totalBytes, nil
		}
		if err != nil {
			return totalBytes, err
		}
		if byteCount == 0 {
			return totalBytes, nil
		}
	}
}

func (b *ReaderAt) Close() error {
	return nil
}

func ReadRange(ctx context.Context, reader io.ReaderFrom, name string, storage objstore.BucketReader, off, size int64) error {
	if size == 0 {
		attrs, err := storage.Attributes(ctx, name)
		if err != nil {
			return err
		}
		size = attrs.Size
	}
	if size == 0 {
		return nil
	}
	rc, err := storage.GetRange(ctx, name, off, size)
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()
	n, err := reader.ReadFrom(io.LimitReader(rc, size))
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("read %d bytes, expected %d", n, size)
	}
	return nil
}

type BucketReaderWithOffset struct {
	BucketReader
	offset int64
}

func NewBucketReaderWithOffset(r BucketReader, offset int64) *BucketReaderWithOffset {
	return &BucketReaderWithOffset{
		BucketReader: r,
		offset:       offset,
	}
}

func (r *BucketReaderWithOffset) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	return r.BucketReader.GetRange(ctx, name, r.offset+off, length)
}
