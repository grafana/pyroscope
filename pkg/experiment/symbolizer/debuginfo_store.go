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
	Enabled bool                  `yaml:"enabled"`
	MaxAge  time.Duration         `yaml:"max_age"`
	Storage objstoreclient.Config `yaml:"storage"`
}

// DebugInfoCache handles caching of debug info files
type DebugInfoStore interface {
	Get(ctx context.Context, buildID string) (io.ReadCloser, error)
	Put(ctx context.Context, buildID string, reader io.Reader) error
	IsEnabled() bool
}

// ObjstoreDebugInfoStore implements DebugInfoStore using object storage
type ObjstoreDebugInfoStore struct {
	bucket  objstore.Bucket
	maxAge  time.Duration
	metrics *Metrics
}

func NewObjstoreDebugInfoStore(bucket objstore.Bucket, maxAge time.Duration, metrics *Metrics) *ObjstoreDebugInfoStore {
	return &ObjstoreDebugInfoStore{
		bucket:  bucket,
		maxAge:  maxAge,
		metrics: metrics,
	}
}

func (c *ObjstoreDebugInfoStore) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	start := time.Now()
	status := StatusSuccess
	defer func() {
		c.metrics.cacheOperations.WithLabelValues("objstore_cache", "get", status).Observe(time.Since(start).Seconds())
	}()

	// First check if object exists to avoid unnecessary operations
	reader, err := c.bucket.Get(ctx, buildID)
	if err != nil {
		if c.bucket.IsObjNotFoundErr(err) {
			status = StatusCacheMiss
			return nil, err
		}
		status = StatusErrorRead
		return nil, fmt.Errorf("get from cache: %w", err)
	}

	// TODO: Implement a separate cleanup job for managing debug info storage
	// We should implement a separate periodic job that:
	// 1. Scans the storage for debug info objects
	// 2. Uses a time-window approach to track access patterns
	// 3. Removes objects that haven't been accessed in any window for a configured period
	// This approach avoids deleting objects based solely on age (which is incorrect for
	// immutable debug info files) and instead focuses on actual usage patterns.
	// When we define this cleanup job, we should:
	// - Make it configurable (window size, cleanup interval...)
	// - Add metrics for tracking cleanup operations

	status = StatusCacheHit
	return reader, nil
}

func (c *ObjstoreDebugInfoStore) Put(ctx context.Context, buildID string, reader io.Reader) error {
	start := time.Now()
	status := StatusSuccess
	defer func() {
		c.metrics.cacheOperations.WithLabelValues("objstore_cache", "put", status).Observe(time.Since(start).Seconds())
	}()

	if err := c.bucket.Upload(ctx, buildID, reader); err != nil {
		status = StatusErrorUpload
		return fmt.Errorf("upload to cache: %w", err)
	}

	return nil
}

// ObjstoreDebugInfoStore implementation
func (o *ObjstoreDebugInfoStore) IsEnabled() bool {
	return true
}

// NullCache implements DebugInfoStore but performs no caching
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

// NullDebugInfoStore implementation
func (n *NullDebugInfoStore) IsEnabled() bool {
	return false
}
