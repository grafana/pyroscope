// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package memory

import (
	"bytes"
	"context"
	"io"

	"github.com/thanos-io/objstore"
)

var _ objstore.Bucket = &InMemBucket{}

// InMemBucket wraps the upstream thanos-io/objstore in-memory bucket
// and adds a Set method so callers that need to exercise conditional
// uploads against an in-memory bucket can do so through this package.
// NOTE: For test use cases only.
type InMemBucket struct {
	*objstore.InMemBucket
}

// NewInMemBucket returns a new in memory Bucket.
func NewInMemBucket() *InMemBucket {
	return &InMemBucket{InMemBucket: objstore.NewInMemBucket()}
}

// Set stores data at name directly as an unconditional Upload
func (b *InMemBucket) Set(name string, data []byte) {
	_ = b.Upload(context.Background(), name, bytes.NewReader(data))
}

// GetRange returns an empty reader for zero-length reads, where the upstream
// bucket returns an error: block readers issue them for empty sections, e.g.
// the symbols of an unsymbolized dataset.
func (b *InMemBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	if length != 0 {
		return b.InMemBucket.GetRange(ctx, name, off, length)
	}
	if _, err := b.Attributes(ctx, name); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}
