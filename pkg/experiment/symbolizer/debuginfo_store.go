package symbolizer

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
)

// DebugInfoStoreConfig holds configuration for the debug info cache
type DebugInfoStoreConfig struct {
	MaxAge  time.Duration         `yaml:"max_age"`
	Storage objstoreclient.Config `yaml:"storage"`
}

// DebugInfoStore handles caching of debug info files
type DebugInfoStore interface {
	Get(ctx context.Context, buildID string) (io.ReadCloser, error)
	Put(ctx context.Context, buildID string, reader io.Reader) error
}

// ObjstoreDebugInfoStore implements DebugInfoStore using object storage
type ObjstoreDebugInfoStore struct {
	bucket  objstore.Bucket
	maxAge  time.Duration
	metrics *metrics
}

func NewObjstoreDebugInfoStore(bucket objstore.Bucket, maxAge time.Duration, metrics *metrics) *ObjstoreDebugInfoStore {
	return &ObjstoreDebugInfoStore{
		bucket:  bucket,
		maxAge:  maxAge,
		metrics: metrics,
	}
}

func (c *ObjstoreDebugInfoStore) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	// First check if object exists to avoid unnecessary operations
	reader, err := c.bucket.Get(ctx, buildID)
	if err != nil {
		if c.bucket.IsObjNotFoundErr(err) {
			return nil, err
		}
		return nil, fmt.Errorf("get from cache: %w", err)
	}

	// TODO: Implement a separate cleanup job for managing debug info storage
	return reader, nil
}

func (c *ObjstoreDebugInfoStore) Put(ctx context.Context, buildID string, reader io.Reader) error {
	return c.bucket.Upload(ctx, buildID, reader)
}

// NullDebugInfoStore implements DebugInfoStore but performs no caching
type NullDebugInfoStore struct{}

// NewNullDebugInfoStore creates a new null debug info store
func NewNullDebugInfoStore() DebugInfoStore {
	return &NullDebugInfoStore{}
}

func (n *NullDebugInfoStore) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("debug info not found")
}

func (n *NullDebugInfoStore) Put(ctx context.Context, buildID string, reader io.Reader) error {
	return nil
}
