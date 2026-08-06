// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package memory

import (
	"bytes"
	"context"

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
