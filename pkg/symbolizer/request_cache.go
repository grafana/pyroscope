package symbolizer

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"
)

// maxRequestCacheBytes bounds the memory held by one request-scoped cache;
// entries beyond the cap are served fetch-through, without being retained.
const maxRequestCacheBytes = 1 << 30 // 1 GiB

const cacheTypeRequest = "request"

type requestCacheContextKey struct{}

// WithRequestCache returns a context carrying a request-scoped cache of
// fetched debug symbol data (lidia bytes), keyed by tenant and build ID,
// so callers symbolizing many profiles in one logical operation fetch each
// binary once per request instead of once per SymbolizePprof call.
func WithRequestCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestCacheContextKey{}, &requestCache{
		items: make(map[string][]byte),
	})
}

func requestCacheFromContext(ctx context.Context) (*requestCache, bool) {
	c, ok := ctx.Value(requestCacheContextKey{}).(*requestCache)
	return c, ok
}

type requestCache struct {
	group singleflight.Group
	mu    sync.Mutex
	items map[string][]byte
	held  int64
}

// getOrFetch returns the cached bytes for key, or fetches and caches them.
// Concurrent calls for the same key share a single fetch; errors are not
// cached, so transient failures don't poison the rest of the request.
func (c *requestCache) getOrFetch(key string, m *metrics, fetch func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	b, ok := c.items[key]
	c.mu.Unlock()
	if ok {
		m.cacheOperations.WithLabelValues(cacheTypeRequest, "get", statusSuccess).Inc()
		return b, nil
	}
	m.cacheOperations.WithLabelValues(cacheTypeRequest, "get", "miss").Inc()
	v, err, _ := c.group.Do(key, func() (interface{}, error) {
		// A previous flight may have populated the cache in the meantime.
		c.mu.Lock()
		b, ok := c.items[key]
		c.mu.Unlock()
		if ok {
			return b, nil
		}
		b, err := fetch()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.held+int64(len(b)) <= maxRequestCacheBytes {
			c.items[key] = b
			c.held += int64(len(b))
		}
		c.mu.Unlock()
		return b, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}
