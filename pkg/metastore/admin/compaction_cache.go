package admin

import (
	"strconv"
	"sync"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// compactionStateCacheTTL bounds how stale the compaction page may be.
//
// Serving the page costs a full scan of the scheduler job queue and of the
// planner block queue, and the block queue is not bounded by anything: it
// grows with the compaction backlog. Triage is exactly the workload that
// repeats those scans, by reloading the page and by walking from the overview
// into a tenant and back, and it happens when the metastore can least afford
// it. A few seconds of staleness costs an operator nothing; the page reports
// how old the state is.
const compactionStateCacheTTL = 5 * time.Second

// compactionStateCacheSize is the number of distinct views kept. The overview
// and a handful of recently visited tenants cover the way the page is walked;
// the responses are large, so the cache is deliberately small.
const compactionStateCacheSize = 4

// compactionStateCache holds recently read compaction state. The zero value
// is a disabled cache: every read goes to the metastore.
type compactionStateCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]compactionStateCacheEntry
}

type compactionStateCacheEntry struct {
	state   *metastorev1.GetCompactionStateResponse
	fetched time.Time
}

// compactionStateCacheKey identifies a view of the compaction state. It must
// distinguish the drill-down of the tenant with no name, which selects the
// multi-tenant segments, from the overview, which selects everything.
func compactionStateCacheKey(drilldown bool, tenantID string) string {
	if !drilldown {
		return "-"
	}
	return strconv.Itoa(len(tenantID)) + ":" + tenantID
}

// get returns the cached state for the key, along with the time it was read.
func (c *compactionStateCache) get(key string, now time.Time) (*metastorev1.GetCompactionStateResponse, time.Time, bool) {
	if c.ttl <= 0 {
		return nil, time.Time{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || now.Sub(entry.fetched) >= c.ttl {
		return nil, time.Time{}, false
	}
	return entry.state, entry.fetched, true
}

func (c *compactionStateCache) put(key string, state *metastorev1.GetCompactionStateResponse, now time.Time) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]compactionStateCacheEntry, compactionStateCacheSize)
	}
	// Drop what has expired before considering the size limit, so that a
	// quiet period does not evict a live entry.
	for k, entry := range c.entries {
		if now.Sub(entry.fetched) >= c.ttl {
			delete(c.entries, k)
		}
	}
	for len(c.entries) >= compactionStateCacheSize {
		var oldestKey string
		var oldest time.Time
		for k, entry := range c.entries {
			if oldest.IsZero() || entry.fetched.Before(oldest) {
				oldestKey, oldest = k, entry.fetched
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = compactionStateCacheEntry{state: state, fetched: now}
}
