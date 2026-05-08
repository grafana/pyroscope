package symbolizer

import (
	"fmt"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"golang.org/x/sync/singleflight"
)

const cacheTypeLidiaInmem = "lidia_inmem"

// Sizes ristretto's TinyLFU sketch (~10x expected items per ristretto docs).
// Multi-MiB lidia files keep practical item counts well under this.
const lidiaCacheNumCounters = 10000

// In-memory cache of lidia bytes keyed by tenant+buildID, with singleflight
// on fill. A nil receiver acts as a disabled cache (passthrough to fetch).
type lidiaCache struct {
	cache  *ristretto.Cache[string, []byte]
	sf     singleflight.Group
	m      *metrics
	logger log.Logger
}

func newLidiaCache(maxBytes int64, m *metrics, logger log.Logger) (*lidiaCache, error) {
	if maxBytes <= 0 {
		return nil, nil
	}
	c, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: lidiaCacheNumCounters,
		MaxCost:     maxBytes,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("create lidia cache: %w", err)
	}
	return &lidiaCache{cache: c, m: m, logger: logger}, nil
}

// getOrFetch returns the cached bytes for key, or invokes fetch on miss.
// Concurrent callers requesting the same key share a single fetch.
func (c *lidiaCache) getOrFetch(key string, fetch func() ([]byte, error)) ([]byte, error) {
	if c == nil {
		return fetch()
	}
	if data, ok := c.cache.Get(key); ok {
		c.m.cacheOperations.WithLabelValues(cacheTypeLidiaInmem, "get", statusSuccess).Inc()
		return data, nil
	}
	c.m.cacheOperations.WithLabelValues(cacheTypeLidiaInmem, "get", "miss").Inc()

	v, err, _ := c.sf.Do(key, func() (any, error) {
		// Re-check: a concurrent caller may have populated the cache.
		if data, ok := c.cache.Get(key); ok {
			return data, nil
		}
		data, err := fetch()
		if err != nil {
			return nil, err
		}
		c.set(key, data)
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

func (c *lidiaCache) set(key string, data []byte) {
	if !c.cache.Set(key, data, int64(len(data))) {
		c.m.cacheOperations.WithLabelValues(cacheTypeLidiaInmem, "set", "rejected").Inc()
		// Most likely cause: item size exceeds the configured cache MaxCost.
		level.Warn(c.logger).Log(
			"msg", "lidia cache rejected entry; item likely exceeds cache size",
			"key", key,
			"item_bytes", len(data),
		)
		return
	}
	c.cache.Wait()
	c.m.cacheOperations.WithLabelValues(cacheTypeLidiaInmem, "set", statusSuccess).Inc()
	c.m.cacheSizeBytes.WithLabelValues(cacheTypeLidiaInmem).
		Set(float64(c.cache.Metrics.CostAdded() - c.cache.Metrics.CostEvicted()))
}

func (c *lidiaCache) close() {
	if c == nil {
		return
	}
	c.cache.Close()
}
