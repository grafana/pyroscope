package querybackend

import (
	"crypto/sha256"
	"math"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
)

// treeResultKey identifies a query result within an immutable block. The
// dataset offset is unique within the object; its size protects against a
// malformed or stale metadata reference to a different dataset at that offset.
type treeResultKey struct {
	object        string
	datasetOffset uint64
	datasetSize   uint64
	requestHash   [sha256.Size]byte
}

func treeResultCacheKey(q *queryContext, query *queryv1.Query) (treeResultKey, bool) {
	if query.GetQueryType() != queryv1.QueryType_QUERY_TREE || query.GetTree() == nil {
		return treeResultKey{}, false
	}
	ds := q.ds.Metadata()
	if len(ds.TableOfContents) == 0 {
		return treeResultKey{}, false
	}
	start, end := q.req.src.StartTime, q.req.src.EndTime
	// The query selects profiles using inclusive nanosecond timestamps, while
	// dataset bounds are milliseconds (the maximum may be truncated from ns).
	// EndTime must therefore exceed MaxTime to guarantee full coverage.
	if ds.MinTime > 0 && ds.MaxTime >= ds.MinTime && ds.MaxTime < math.MaxInt64 &&
		end >= ds.MinTime && start <= ds.MaxTime {
		start = max(start, ds.MinTime)
		end = min(end, ds.MaxTime+1)
	}
	// QueryPlan, diagnostics and the other queries in the same invocation do
	// not affect this dataset's tree result. Windows that fully cover the same
	// immutable dataset share a key; partial windows remain distinct. Everything
	// else used by the tree query and profile-entry selection is included. Keep
	// the full Options message so future options cannot accidentally alias an
	// existing cache entry.
	req := &queryv1.InvokeRequest{
		Tenant:        q.req.src.Tenant,
		StartTime:     start,
		EndTime:       end,
		LabelSelector: q.req.src.LabelSelector,
		Query:         []*queryv1.Query{query},
		Options:       q.req.src.Options,
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(req)
	if err != nil {
		return treeResultKey{}, false
	}
	return treeResultKey{
		object:        block.ObjectPath(q.obj.Metadata()),
		datasetOffset: ds.TableOfContents[0],
		datasetSize:   ds.Size,
		requestHash:   sha256.Sum256(b),
	}, true
}

// treeResultCache is a process-local, byte-bounded LRU. Values are immutable
// protobuf bytes; each hit decodes a new report because aggregation may mutate
// the returned tree. Its budget covers payload bytes, not Go map/LRU overhead.
type treeResultCache struct {
	mu       sync.Mutex
	lru      *lru.Cache[treeResultKey, []byte]
	maxBytes int64
	bytes    int64
	metrics  *treeResultCacheMetrics
}

func newTreeResultCache(maxBytes int64, reg prometheus.Registerer) *treeResultCache {
	if maxBytes <= 0 {
		return nil
	}
	c := &treeResultCache{maxBytes: maxBytes, metrics: newTreeResultCacheMetrics(reg)}
	c.lru, _ = lru.NewWithEvict[treeResultKey, []byte](1<<20, func(_ treeResultKey, v []byte) {
		c.bytes -= int64(len(v))
		c.metrics.evictions.Inc()
	})
	return c
}

func (c *treeResultCache) get(k treeResultKey) (*queryv1.Report, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	b, ok := c.lru.Get(k)
	if ok {
		c.metrics.hits.Inc()
	} else {
		c.metrics.misses.Inc()
	}
	c.mu.Unlock()
	if !ok {
		return nil, false
	}
	r := new(queryv1.Report)
	if err := r.UnmarshalVT(b); err != nil {
		// A malformed entry must never change query behavior.
		c.mu.Lock()
		c.lru.Remove(k)
		c.metrics.sizeBytes.Set(float64(c.bytes))
		c.metrics.entries.Set(float64(c.lru.Len()))
		c.mu.Unlock()
		return nil, false
	}
	return r, true
}

func (c *treeResultCache) add(k treeResultKey, r *queryv1.Report) {
	if c == nil || r == nil {
		return
	}
	b, err := r.MarshalVT()
	if err != nil {
		return
	}
	if int64(len(b)) > c.maxBytes {
		c.metrics.oversize.Inc()
		return
	}
	c.mu.Lock()
	if old, exists := c.lru.Peek(k); exists {
		c.bytes -= int64(len(old))
	}
	c.bytes += int64(len(b))
	c.lru.Add(k, b)
	for c.bytes > c.maxBytes {
		c.lru.RemoveOldest()
	}
	c.metrics.sizeBytes.Set(float64(c.bytes))
	c.metrics.entries.Set(float64(c.lru.Len()))
	c.mu.Unlock()
}

type treeResultCacheMetrics struct {
	hits      prometheus.Counter
	misses    prometheus.Counter
	evictions prometheus.Counter
	oversize  prometheus.Counter
	sizeBytes prometheus.Gauge
	entries   prometheus.Gauge
}

func newTreeResultCacheMetrics(reg prometheus.Registerer) *treeResultCacheMetrics {
	m := &treeResultCacheMetrics{
		hits: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_query_backend_tree_result_cache_hits_total",
			Help: "Successful per-dataset tree-result cache lookups.",
		}),
		misses: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_query_backend_tree_result_cache_misses_total",
			Help: "Per-dataset tree-result cache lookups without an entry.",
		}),
		evictions: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_query_backend_tree_result_cache_evictions_total",
			Help: "Per-dataset tree-result cache entries removed from the LRU.",
		}),
		oversize: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_query_backend_tree_result_cache_oversize_total",
			Help: "Per-dataset tree results too large for the configured cache budget.",
		}),
		sizeBytes: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_query_backend_tree_result_cache_size_bytes",
			Help: "Serialized payload bytes retained by the per-dataset tree-result cache.",
		}),
		entries: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_query_backend_tree_result_cache_entries",
			Help: "Number of per-dataset tree results retained in the cache.",
		}),
	}
	return m
}
