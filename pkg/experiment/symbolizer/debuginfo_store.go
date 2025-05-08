package symbolizer

import (
	"context"
	"fmt"
	"io"

	"github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
)

// DebugInfoStoreConfig holds configuration for the debug info cache
type DebugInfoStoreConfig struct {
	Storage objstoreclient.Config `yaml:"storage"`
}

// DebugInfoStore handles caching of debug info files
type DebugInfoStore interface {
	Get(ctx context.Context, buildID string) (io.ReadCloser, error)
	Put(ctx context.Context, buildID string, reader io.Reader) error
}

// ObjstoreDebugInfoStore implements DebugInfoStore using object storage
type ObjstoreDebugInfoStore struct {
	bucket objstore.Bucket
}

func NewObjstoreDebugInfoStore(bucket objstore.Bucket) *ObjstoreDebugInfoStore {
	return &ObjstoreDebugInfoStore{
		bucket: bucket,
	}
}

func (c *ObjstoreDebugInfoStore) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	reader, err := c.bucket.Get(ctx, buildID)
	if err != nil {
		if c.bucket.IsObjNotFoundErr(err) {
			return nil, err
		}
		return nil, fmt.Errorf("get from cache: %w", err)
	}

	return reader, nil
}

func (c *ObjstoreDebugInfoStore) Put(ctx context.Context, buildID string, reader io.Reader) error {
	return c.bucket.Upload(ctx, buildID, reader)
}
