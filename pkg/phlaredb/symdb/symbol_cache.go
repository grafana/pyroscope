package symdb

import (
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

// SymbolCache is a process-level, byte-bounded LRU cache of decoded symbol
// tables (locations, mappings, functions, strings), keyed by (block ULID,
// partition). It lets the store-gateway decode a partition's symbols once and
// reuse them across queries instead of re-decoding from object storage every
// time. It is shared across all blocks and tenants, owned by the store-gateway
// process (constructed in storegateway.NewBucketStores) and threaded into each
// block's symdb.Reader via WithSymbolCache.
//
// Bounding: curBytes tracks the (estimated) bytes of the entries resident in the
// cache. Eviction always drives it back to <= maxBytes (the LRU can always evict
// its oldest entry), so the cache's own resident set is bounded. Two things make
// this a soft rather than hard bound on process memory: (1) the per-entry byte
// cost is an estimate, and (2) an entry evicted while a query is still using it
// lingers until that query drops its reference (bounded by in-flight queries).
// GOMEMLIMIT remains the hard backstop for the process.
//
// Concurrency: mu guards all mutable state (the LRU and curBytes). All methods
// take mu; onEvict is only ever invoked synchronously from lru.Add/Remove calls
// made while mu is held, so it does not re-lock. The decoded slices inside a
// cachedSymbols are immutable after construction, so callers read them without
// holding mu — and an entry evicted mid-use stays alive via the caller's
// reference (GC), so eviction is never a use-after-free.
type SymbolCache struct {
	mu       sync.Mutex
	lru      *lru.Cache[SymbolCacheKey, *cachedSymbols]
	maxBytes int64
	curBytes int64
	metrics  *symbolCacheMetrics
}

// SymbolCacheKey identifies a whole decoded partition within a block.
type SymbolCacheKey struct {
	Block     ulid.ULID
	Partition uint64
}

// cachedSymbols holds the four decoded symbol slices for one partition. The
// slices are immutable and safe to share across concurrent readers.
type cachedSymbols struct {
	locations []schemav1.InMemoryLocation
	mappings  []schemav1.InMemoryMapping
	functions []schemav1.InMemoryFunction
	strings   []string
	bytes     int64
}

// Byte-cost constants (see pkg/phlaredb/schemas/v1). Estimates for budgeting.
const (
	sizeInMemoryLocationFixed = 56 // struct incl. []InMemoryLine slice header
	sizeInMemoryLine          = 8  // FunctionId uint32 + Line int32
	sizeInMemoryMapping       = 48
	sizeInMemoryFunction      = 24
	sizeStringHeader          = 16
	sizeCachedSymbolsOverhead = 96
	// lruEntryCap is a large upper bound on entry count; the real bound is the
	// byte budget enforced by evictToBudgetLocked.
	lruEntryCap = 1 << 20
)

func newCachedSymbols(
	locs []schemav1.InMemoryLocation,
	maps []schemav1.InMemoryMapping,
	fns []schemav1.InMemoryFunction,
	strs []string,
) *cachedSymbols {
	b := int64(sizeCachedSymbolsOverhead)
	b += int64(len(locs)) * sizeInMemoryLocationFixed
	for i := range locs {
		b += int64(len(locs[i].Line)) * sizeInMemoryLine
	}
	b += int64(len(maps)) * sizeInMemoryMapping
	b += int64(len(fns)) * sizeInMemoryFunction
	for i := range strs {
		b += sizeStringHeader + int64(len(strs[i]))
	}
	return &cachedSymbols{locations: locs, mappings: maps, functions: fns, strings: strs, bytes: b}
}

// NewSymbolCache returns a cache bounded by maxBytes, or nil if maxBytes <= 0
// (disabled). A nil *SymbolCache is safe: all methods are no-ops and the reader
// takes a zero-overhead fast path. Metrics are registered only when enabled.
func NewSymbolCache(maxBytes int64, reg prometheus.Registerer) *SymbolCache {
	if maxBytes <= 0 {
		return nil
	}
	c := &SymbolCache{maxBytes: maxBytes, metrics: newSymbolCacheMetrics(reg)}
	c.lru, _ = lru.NewWithEvict[SymbolCacheKey, *cachedSymbols](lruEntryCap, c.onEvict)
	return c
}

// get returns the cached symbols for key, or nil on miss.
func (c *SymbolCache) get(k SymbolCacheKey) *cachedSymbols {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cs, ok := c.lru.Get(k)
	if !ok {
		c.metrics.incMisses()
		return nil
	}
	c.metrics.incHits()
	return cs
}

// add inserts a freshly decoded entry and evicts oldest entries to stay within
// the byte budget. If the key already exists, lru.Add replaces it and onEvict
// reconciles the byte accounting.
func (c *SymbolCache) add(k SymbolCacheKey, cs *cachedSymbols) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.curBytes += cs.bytes
	c.lru.Add(k, cs)
	c.evictToBudgetLocked()
	c.metrics.setSize(c.curBytes, c.lru.Len())
}

// PurgeBlock removes all entries for a block, called on block drop via
// Reader.Close to proactively reclaim its budget (the block will never be
// queried again — ULIDs are unique). Not required for correctness: absent this,
// the entries are cold and age out via LRU. Runs off the query hot path (once
// per dropped block on the ~15m sync), so its O(n) key scan is acceptable.
func (c *SymbolCache) PurgeBlock(id ulid.ULID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range c.lru.Keys() {
		if k.Block == id {
			c.lru.Remove(k) // triggers onEvict → curBytes adjusted
		}
	}
	c.metrics.setSize(c.curBytes, c.lru.Len())
}

// onEvict is invoked by the LRU (always under c.mu) when an entry is removed or
// replaced.
func (c *SymbolCache) onEvict(_ SymbolCacheKey, cs *cachedSymbols) {
	c.curBytes -= cs.bytes
	c.metrics.incEvictions()
}

// evictToBudgetLocked removes least-recently-used entries until within budget.
func (c *SymbolCache) evictToBudgetLocked() {
	for c.curBytes > c.maxBytes {
		if _, _, ok := c.lru.RemoveOldest(); !ok {
			return // empty
		}
	}
}

// symbolCacheMetrics holds cache observability. All methods are nil-safe.
type symbolCacheMetrics struct {
	hits      prometheus.Counter
	misses    prometheus.Counter
	evictions prometheus.Counter
	sizeBytes prometheus.Gauge
	entries   prometheus.Gauge
}

func newSymbolCacheMetrics(reg prometheus.Registerer) *symbolCacheMetrics {
	return &symbolCacheMetrics{
		hits: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_store_gateway_symbol_cache_hits_total",
			Help: "Total number of decoded-symbol cache hits.",
		}),
		misses: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_store_gateway_symbol_cache_misses_total",
			Help: "Total number of decoded-symbol cache misses.",
		}),
		evictions: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_store_gateway_symbol_cache_evictions_total",
			Help: "Total number of decoded-symbol cache entries evicted.",
		}),
		sizeBytes: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_store_gateway_symbol_cache_size_bytes",
			Help: "Current estimated size of the decoded-symbol cache in bytes.",
		}),
		entries: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_store_gateway_symbol_cache_entries",
			Help: "Current number of entries in the decoded-symbol cache.",
		}),
	}
}

func (m *symbolCacheMetrics) incHits() {
	if m != nil {
		m.hits.Inc()
	}
}

func (m *symbolCacheMetrics) incMisses() {
	if m != nil {
		m.misses.Inc()
	}
}

func (m *symbolCacheMetrics) incEvictions() {
	if m != nil {
		m.evictions.Inc()
	}
}

func (m *symbolCacheMetrics) setSize(bytes int64, entries int) {
	if m != nil {
		m.sizeBytes.Set(float64(bytes))
		m.entries.Set(float64(entries))
	}
}
