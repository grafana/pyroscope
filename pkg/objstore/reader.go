package objstore

import (
	"context"
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
