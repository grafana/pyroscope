package objstore

// NewBorrowedBucket returns a view whose Close does not close the underlying
// bucket. The caller retains ownership and must close it after all borrowers
// have stopped, including users of prefixed and SSE wrappers.
func NewBorrowedBucket(bucket Bucket) InstrumentedBucket {
	return &borrowedBucket{Bucket: bucket}
}

type borrowedBucket struct{ Bucket }

func (*borrowedBucket) Close() error { return nil }

func (b *borrowedBucket) WithExpectedErrs(fn IsOpFailureExpectedFunc) Bucket {
	if bucket, ok := b.Bucket.(InstrumentedBucket); ok {
		return NewBorrowedBucket(bucket.WithExpectedErrs(fn))
	}
	return b
}

func (b *borrowedBucket) ReaderWithExpectedErrs(fn IsOpFailureExpectedFunc) BucketReader {
	return b.WithExpectedErrs(fn)
}

func (b *borrowedBucket) Prefix() string {
	if bucket, ok := b.Bucket.(interface{ Prefix() string }); ok {
		return bucket.Prefix()
	}
	return ""
}
