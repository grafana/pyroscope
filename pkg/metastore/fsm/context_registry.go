package fsm

import (
	"context"
	"sync"
	"time"
)

// ContextRegistry maintains a mapping of Raft log indices to contexts for tracing purposes.
// This allows us to propagate tracing context from HTTP handlers down to BoltDB transactions
// without persisting the context in Raft logs.
//
// The registry is only used on the leader node where Propose() is called. Followers will
// not have contexts available and will use context.Background() instead.
type ContextRegistry struct {
	mu      sync.RWMutex
	entries map[uint64]*contextEntry
	// cleanupInterval determines how often we scan for expired entries
	cleanupInterval time.Duration
	// entryTTL is the maximum age of an entry before it's considered expired
	entryTTL time.Duration
	stop     chan struct{}
	done     chan struct{}
}

type contextEntry struct {
	ctx       context.Context
	timestamp time.Time
}

// NewContextRegistry creates a new context registry with background cleanup.
func NewContextRegistry(cleanupInterval, entryTTL time.Duration) *ContextRegistry {
	if cleanupInterval <= 0 {
		cleanupInterval = 10 * time.Second
	}
	if entryTTL <= 0 {
		entryTTL = 30 * time.Second
	}

	r := &ContextRegistry{
		entries:         make(map[uint64]*contextEntry),
		cleanupInterval: cleanupInterval,
		entryTTL:        entryTTL,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}

	go r.cleanupLoop()
	return r
}

// Store saves a context for the given Raft log index.
func (r *ContextRegistry) Store(index uint64, ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[index] = &contextEntry{
		ctx:       ctx,
		timestamp: time.Now(),
	}
}

// Retrieve gets the context for the given Raft log index.
// If no context is found, it returns context.Background() and false.
func (r *ContextRegistry) Retrieve(index uint64) (context.Context, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, ok := r.entries[index]; ok {
		return entry.ctx, true
	}
	return context.Background(), false
}

// Delete removes the context for the given Raft log index.
func (r *ContextRegistry) Delete(index uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, index)
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
	for index, entry := range r.entries {
		if now.Sub(entry.timestamp) > r.entryTTL {
			delete(r.entries, index)
		}
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
