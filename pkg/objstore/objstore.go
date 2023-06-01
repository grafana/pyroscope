// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package objstore

import (
	"context"
	"io"
	"strings"

	"github.com/thanos-io/objstore"
)

type Bucket interface {
	objstore.Bucket
	ReaderAt(ctx context.Context, filename string) (ReaderAtCloser, error)
}

type BucketReader interface {
	objstore.BucketReader
	ReaderAt(ctx context.Context, filename string) (ReaderAtCloser, error)
}

type PrefixedBucket struct {
	Bucket
	prefix string
}

func NewPrefixedBucket(bkt Bucket, prefix string) Bucket {
	if validPrefix(prefix) {
		return &PrefixedBucket{Bucket: bkt, prefix: strings.Trim(prefix, objstore.DirDelim)}
	}

	return bkt
}

func validPrefix(prefix string) bool {
	prefix = strings.Replace(prefix, "/", "", -1)
	return len(prefix) > 0
}

func conditionalPrefix(prefix, name string) string {
	if len(name) > 0 {
		return withPrefix(prefix, name)
	}

	return name
}

func withPrefix(prefix, name string) string {
	return prefix + objstore.DirDelim + name
}

func (p *PrefixedBucket) Close() error {
	return p.Bucket.Close()
}

func (p *PrefixedBucket) Prefix() string {
	if prefixed, ok := p.Bucket.(interface{ Prefix() string }); ok && prefixed.Prefix() != "" {
		return prefixed.Prefix() + objstore.DirDelim + p.prefix
	}
	return p.prefix
}

func (p *PrefixedBucket) ReaderAt(ctx context.Context, name string) (ReaderAtCloser, error) {
	return p.Bucket.ReaderAt(ctx, conditionalPrefix(p.prefix, name))
}

// Iter calls f for each entry in the given directory (not recursive.). The argument to f is the full
// object name including the prefix of the inspected directory.
// Entries are passed to function in sorted order.
func (p *PrefixedBucket) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	pdir := withPrefix(p.prefix, dir)

	return p.Bucket.Iter(ctx, pdir, func(s string) error {
		return f(strings.TrimPrefix(s, p.prefix+objstore.DirDelim))
	}, options...)
}

// Get returns a reader for the given object name.
func (p *PrefixedBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return p.Bucket.Get(ctx, conditionalPrefix(p.prefix, name))
}

// GetRange returns a new range reader for the given object name and range.
func (p *PrefixedBucket) GetRange(ctx context.Context, name string, off int64, length int64) (io.ReadCloser, error) {
	return p.Bucket.GetRange(ctx, conditionalPrefix(p.prefix, name), off, length)
}

// Exists checks if the given object exists in the bucket.
func (p *PrefixedBucket) Exists(ctx context.Context, name string) (bool, error) {
	return p.Bucket.Exists(ctx, conditionalPrefix(p.prefix, name))
}

// IsObjNotFoundErr returns true if error means that object is not found. Relevant to Get operations.
func (p *PrefixedBucket) IsObjNotFoundErr(err error) bool {
	return p.Bucket.IsObjNotFoundErr(err)
}

// Attributes returns information about the specified object.
func (p PrefixedBucket) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	return p.Bucket.Attributes(ctx, conditionalPrefix(p.prefix, name))
}

// Upload the contents of the reader as an object into the bucket.
// Upload should be idempotent.
func (p *PrefixedBucket) Upload(ctx context.Context, name string, r io.Reader) error {
	return p.Bucket.Upload(ctx, conditionalPrefix(p.prefix, name), r)
}

// Delete removes the object with the given name.
// If object does not exists in the moment of deletion, Delete should throw error.
func (p *PrefixedBucket) Delete(ctx context.Context, name string) error {
	return p.Bucket.Delete(ctx, conditionalPrefix(p.prefix, name))
}

// Name returns the bucket name for the provider.
func (p *PrefixedBucket) Name() string {
	return p.Bucket.Name()
}
