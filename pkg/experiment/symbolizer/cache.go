package symbolizer

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/grafana/pyroscope/pkg/objstore"
)

// CacheConfig holds configuration for the debug info cache
type CacheConfig struct {
	Enabled bool          `yaml:"enabled"`
	MaxAge  time.Duration `yaml:"max_age"`
}

func NewObjstoreCache(bucket objstore.Bucket, maxAge time.Duration, metrics *Metrics) *ObjstoreCache {
	return &ObjstoreCache{
		bucket:  bucket,
		maxAge:  maxAge,
		metrics: metrics,
	}
}

// DebugInfoCache handles caching of debug info files
type DebugInfoCache interface {
	Get(ctx context.Context, buildID string) (io.ReadCloser, error)
	Put(ctx context.Context, buildID string, reader io.Reader) error
}

// ObjstoreCache implements DebugInfoCache using S3 storage
type ObjstoreCache struct {
	bucket  objstore.Bucket
	maxAge  time.Duration
	metrics *Metrics
}

func (c *ObjstoreCache) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	c.metrics.cacheRequestsTotal.WithLabelValues("get").Inc()
	start := time.Now()
	defer func() {
		c.metrics.cacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())
	}()

	// First check if object exists to avoid unnecessary operations
	reader, err := c.bucket.Get(ctx, buildID)
	if err != nil {
		if c.bucket.IsObjNotFoundErr(err) {
			c.metrics.cacheMissesTotal.Inc()
			return nil, err
		}
		c.metrics.cacheRequestErrorsTotal.WithLabelValues("get", "read_error").Inc()
		return nil, fmt.Errorf("get from cache: %w", err)
	}

	// Get attributes - this should use the same HEAD request that Get used
	attrs, err := c.bucket.Attributes(ctx, buildID)
	if err != nil {
		reader.Close()
		c.metrics.cacheRequestErrorsTotal.WithLabelValues("get", "attribute_error").Inc()
		return nil, fmt.Errorf("get cache attributes: %w", err)
	}

	// Check if expired
	if time.Since(attrs.LastModified) > c.maxAge {
		reader.Close()
		c.metrics.cacheExpiredTotal.Inc()

		// Async deletion to not block the request
		go func() {
			delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err = c.bucket.Delete(delCtx, buildID); err != nil {
				c.metrics.cacheRequestErrorsTotal.WithLabelValues("delete", "delete_error").Inc()
			}
		}()
		return nil, fmt.Errorf("cached object expired")
	}

	c.metrics.cacheHitsTotal.Inc()
	return reader, nil
}

func (c *ObjstoreCache) Put(ctx context.Context, buildID string, reader io.Reader) error {
	c.metrics.cacheRequestsTotal.WithLabelValues("put").Inc()
	start := time.Now()
	defer func() {
		c.metrics.cacheOperationDuration.WithLabelValues("put").Observe(time.Since(start).Seconds())
	}()

	if err := c.bucket.Upload(ctx, buildID, reader); err != nil {
		c.metrics.cacheRequestErrorsTotal.WithLabelValues("put", "upload_error").Inc()
		return fmt.Errorf("upload to cache: %w", err)
	}

	return nil
}

// NullCache implements DebugInfoCache but performs no caching
type NullCache struct{}

func NewNullCache() DebugInfoCache {
	return &NullCache{}
}

func (n *NullCache) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("cache miss")
}

func (n *NullCache) Put(ctx context.Context, buildID string, reader io.Reader) error {
	return nil
}
