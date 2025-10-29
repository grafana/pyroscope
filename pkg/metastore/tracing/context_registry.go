package tracing

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

// ContextRegistry maintains a mapping of IDs to contexts for tracing purposes.
// This allows us to propagate tracing context from HTTP handlers down to BoltDB transactions
// without persisting the context in Raft logs.
//
// The registry is only used on the leader node where Propose() is called. Followers will
// not have contexts available and will use context.Background() instead.
type ContextRegistry struct {
	mu      sync.RWMutex
	entries map[string]*contextEntry
	// cleanupInterval determines how often we scan for expired entries
	cleanupInterval time.Duration
	// entryTTL is the maximum age of an entry before it's considered expired
	entryTTL time.Duration
	stop     chan struct{}
	done     chan struct{}

	// sizeMetric tracks the number of entries in the registry
	sizeMetric prometheus.Gauge
}

type contextEntry struct {
	ctx     context.Context
	created time.Time
}

const (
	defaultCleanupInterval = 10 * time.Second
	defaultEntryTTL        = 30 * time.Second
)

// NewContextRegistry creates a new context registry with background cleanup.
func NewContextRegistry(reg prometheus.Registerer) *ContextRegistry {
	return newContextRegistry(defaultCleanupInterval, defaultEntryTTL, reg)
}

// newContextRegistry creates a new context registry with background cleanup.
func newContextRegistry(cleanupInterval, entryTTL time.Duration, reg prometheus.Registerer) *ContextRegistry {
	if cleanupInterval <= 0 {
		cleanupInterval = defaultCleanupInterval
	}
	if entryTTL <= 0 {
		entryTTL = defaultEntryTTL
	}

	sizeMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "context_registry_size",
		Help: "Number of contexts currently stored in the registry for tracing propagation",
	})
	if reg != nil {
		util.RegisterOrGet(reg, sizeMetric)
	}

	r := &ContextRegistry{
		entries:         make(map[string]*contextEntry),
		cleanupInterval: cleanupInterval,
		entryTTL:        entryTTL,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
		sizeMetric:      sizeMetric,
	}

	go r.cleanupLoop()
	return r
}

// Store saves a context for the given ID.
func (r *ContextRegistry) Store(id string, ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[id] = &contextEntry{
		ctx:     ctx,
		created: time.Now(),
	}
}

// Retrieve gets the context for the given ID.
func (r *ContextRegistry) Retrieve(id string) (context.Context, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, ok := r.entries[id]; ok {
		return entry.ctx, true
	}
	return context.Background(), false
}

// Delete removes the context for the given ID.
func (r *ContextRegistry) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, id)
}

// cleanupLoop periodically removes expired entries from the registry.
func (r *ContextRegistry) cleanupLoop() {
	defer close(r.done)
	ticker := time.NewTicker(r.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.cleanup()
		case <-r.stop:
			return
		}
	}
}

// cleanup removes entries that are older than the TTL.
func (r *ContextRegistry) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, entry := range r.entries {
		if now.Sub(entry.created) > r.entryTTL {
			delete(r.entries, id)
		}
	}

	if r.sizeMetric != nil {
		r.sizeMetric.Set(float64(len(r.entries)))
	}
}

// Shutdown stops the cleanup loop.
func (r *ContextRegistry) Shutdown() {
	close(r.stop)
	<-r.done
}

// Size returns the current number of entries in the registry.
// This is primarily useful for metrics and testing.
func (r *ContextRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
