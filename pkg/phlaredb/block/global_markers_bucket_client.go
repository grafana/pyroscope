// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/markers_bucket_client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package block

import (
	"bytes"
	"context"
	"io"
	"path"

	"github.com/grafana/dskit/multierror"
	"github.com/oklog/ulid"
	thanosobjstore "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/pkg/objstore"
)

// globalMarkersBucket is a bucket client which stores markers (eg. block deletion marks) in a per-tenant
// global location too.
type globalMarkersBucket struct {
	parent objstore.Bucket
}

// BucketWithGlobalMarkers wraps the input bucket into a bucket which also keeps track of markers
// in the global markers location.
func BucketWithGlobalMarkers(b objstore.Bucket) objstore.Bucket {
	return &globalMarkersBucket{
		parent: b,
	}
}

// Upload implements objstore.Bucket.
func (b *globalMarkersBucket) Upload(ctx context.Context, name string, r io.Reader) error {
	globalMarkPath := getGlobalMarkPathFromBlockMark(name)
	if globalMarkPath == "" {
		return b.parent.Upload(ctx, name, r)
	}

	// Read the marker.
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// Upload it to the original location.
	if err := b.parent.Upload(ctx, name, bytes.NewBuffer(body)); err != nil {
		return err
	}

	// Upload it to the global markers location too.
	return b.parent.Upload(ctx, globalMarkPath, bytes.NewBuffer(body))
}

// Delete implements objstore.Bucket.
func (b *globalMarkersBucket) Delete(ctx context.Context, name string) error {
	// Call the parent. Only return error here (without deleting global marker too) if error is different than "not found".
	err1 := b.parent.Delete(ctx, name)
	if err1 != nil && !b.parent.IsObjNotFoundErr(err1) {
		return err1
	}

	// Delete the marker in the global markers location too.
	globalMarkPath := getGlobalMarkPathFromBlockMark(name)
	if globalMarkPath == "" {
		return err1
	}

	var err2 error
	if err := b.parent.Delete(ctx, globalMarkPath); err != nil {
		if !b.parent.IsObjNotFoundErr(err) {
			err2 = err
		}
	}

	if err1 != nil {
		// In this case err1 is "ObjNotFound". If we tried to wrap it together with err2, we would need to
		// handle this possibility in globalMarkersBucket.IsObjNotFoundErr(). Instead we just ignore err2, if any.
		return err1
	}

	return err2
}

// Name implements objstore.Bucket.
func (b *globalMarkersBucket) Name() string {
	return b.parent.Name()
}

// Close implements objstore.Bucket.
func (b *globalMarkersBucket) Close() error {
	return b.parent.Close()
}

// Iter implements objstore.Bucket.
func (b *globalMarkersBucket) Iter(ctx context.Context, dir string, f func(string) error, options ...thanosobjstore.IterOption) error {
	return b.parent.Iter(ctx, dir, f, options...)
}

// Get implements objstore.Bucket.
func (b *globalMarkersBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.parent.Get(ctx, name)
}

// GetRange implements objstore.Bucket.
func (b *globalMarkersBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	return b.parent.GetRange(ctx, name, off, length)
}

// Exists implements objstore.Bucket.
func (b *globalMarkersBucket) Exists(ctx context.Context, name string) (bool, error) {
	globalMarkPath := getGlobalMarkPathFromBlockMark(name)
	if globalMarkPath == "" {
		return b.parent.Exists(ctx, name)
	}

	// Report "exists" only if BOTH (block-local, and global) files exist, otherwise Thanos
	// code will never try to upload the file again, if it finds that it exist.
	ok1, err1 := b.parent.Exists(ctx, name)
	ok2, err2 := b.parent.Exists(ctx, globalMarkPath)

	var me multierror.MultiError
	me.Add(err1)
	me.Add(err2)

	return ok1 && ok2, me.Err()
}

// IsObjNotFoundErr implements objstore.Bucket.
func (b *globalMarkersBucket) IsObjNotFoundErr(err error) bool {
	return b.parent.IsObjNotFoundErr(err)
}

// IsAccessDeniedErr returns true if acces to object is denied.
func (b *globalMarkersBucket) IsAccessDeniedErr(err error) bool {
	return b.parent.IsAccessDeniedErr(err)
}

// Attributes implements objstore.Bucket.
func (b *globalMarkersBucket) Attributes(ctx context.Context, name string) (thanosobjstore.ObjectAttributes, error) {
	return b.parent.Attributes(ctx, name)
}

// Attributes implements objstore.ReaderAt.
func (b *globalMarkersBucket) ReaderAt(ctx context.Context, filename string) (objstore.ReaderAtCloser, error) {
	return b.parent.ReaderAt(ctx, filename)
}

// ReaderWithExpectedErrs implements objstore.Bucket.
func (b *globalMarkersBucket) ReaderWithExpectedErrs(fn objstore.IsOpFailureExpectedFunc) objstore.BucketReader {
	return b.WithExpectedErrs(fn)
}

// WithExpectedErrs implements objstore.Bucket.
func (b *globalMarkersBucket) WithExpectedErrs(fn objstore.IsOpFailureExpectedFunc) objstore.Bucket {
	if ib, ok := b.parent.(objstore.InstrumentedBucket); ok {
		return &globalMarkersBucket{
			parent: ib.WithExpectedErrs(fn),
		}
	}

	return b
}

// getGlobalMarkPathFromBlockMark returns path to global mark, if name points to a block-local mark file. If name
// doesn't point to a block-local mark file, returns empty string.
func getGlobalMarkPathFromBlockMark(name string) string {
	if blockID, ok := isDeletionMark(name); ok {
		return path.Clean(path.Join(path.Dir(name), "../", DeletionMarkFilepath(blockID)))
	}

	if blockID, ok := isNoCompactMark(name); ok {
		return path.Clean(path.Join(path.Dir(name), "../", NoCompactMarkFilepath(blockID)))
	}

	return ""
}

func isDeletionMark(name string) (ulid.ULID, bool) {
	if path.Base(name) != DeletionMarkFilename {
		return ulid.ULID{}, false
	}

	// Parse the block ID in the path. If there's no block ID, then it's not the per-block
	// deletion mark.
	return IsBlockDir(path.Dir(name))
}

func isNoCompactMark(name string) (ulid.ULID, bool) {
	if path.Base(name) != NoCompactMarkFilename {
		return ulid.ULID{}, false
	}

	// Parse the block ID in the path. If there's no block ID, then it's not the per-block
	// no-compact mark.
	return IsBlockDir(path.Dir(name))
}
