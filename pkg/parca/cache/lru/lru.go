// Copyright 2023-2025 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//nolint:forcetypeassert,nonamedreturns
package lru

import (
	"container/list"

	"github.com/prometheus/client_golang/prometheus"
)

type entry[K comparable, V any] struct {
	key   K
	value V
}

type LRU[K comparable, V any] struct {
	metrics *metrics
	closer  func() error

	maxEntries int // Zero means no limit.
	onEvicted  func(K, V)
	onAdded    func(K, V)

	evictList *list.List
	items     map[K]*list.Element
}

// New returns a new cache with the provided maximum items count.
func New[K comparable, V any](reg prometheus.Registerer, opts ...Option[K, V]) *LRU[K, V] {
	m := newMetrics(reg)

	lru := &LRU[K, V]{
		metrics: m,
		closer:  m.unregister,

		evictList: list.New(),
		items:     make(map[K]*list.Element),
	}

	for _, opt := range opts {
		opt(lru)
	}
	return lru
}

// Add adds a value to the cache.
func (c *LRU[K, V]) Add(key K, value V) {
	if e, ok := c.items[key]; ok {
		c.evictList.MoveToFront(e)
		e.Value = entry[K, V]{key, value}
		return
	}

	e := c.evictList.PushFront(entry[K, V]{key, value})
	c.items[key] = e

	if c.maxEntries != 0 && c.evictList.Len() > c.maxEntries {
		c.removeOldest()
	}

	if c.onAdded != nil {
		c.onAdded(key, value)
	}
}

// Get looks up a key's value from the cache.
func (c *LRU[K, V]) Get(key K) (value V, ok bool) {
	if e, ok := c.items[key]; ok {
		c.evictList.MoveToFront(e)
		c.metrics.hits.Inc()
		return e.Value.(entry[K, V]).value, true
	}
	c.metrics.misses.Inc()
	return value, ok
}

// Peek returns the key value (or undefined if not found) without updating the "recently used"-ness of the key.
func (c *LRU[K, V]) Peek(key K) (value V, ok bool) {
	if e, ok := c.items[key]; ok {
		return e.Value.(entry[K, V]).value, true
	}
	return value, ok
}

// Remove removes the provided key from the cache.
func (c *LRU[K, V]) Remove(key K) {
	if e, ok := c.items[key]; ok {
		c.removeElement(e)
	}
}

// removeOldest removes the oldest item from the cache.
func (c *LRU[K, V]) removeOldest() {
	e := c.evictList.Back()
	if e != nil {
		c.removeElement(e)
	}
}

// removeElement is used to remove a given list element from the cache.
func (c *LRU[K, V]) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	kv := e.Value.(entry[K, V])
	delete(c.items, kv.key)
	if c.onEvicted != nil {
		c.onEvicted(kv.key, kv.value)
	}
	c.metrics.evictions.Inc()
}

// Purge is used to completely clear the cache.
func (c *LRU[K, V]) Purge() {
	for k, e := range c.items {
		if c.onEvicted != nil {
			c.onEvicted(k, e.Value.(entry[K, V]).value)
		}
		delete(c.items, k)
	}
	c.evictList.Init()
}

// Close closes the cache using registered closer.
func (c *LRU[K, V]) Close() error {
	c.Purge()
	if c.closer != nil {
		return c.closer()
	}
	return nil
}

// removeMatching removes items from the cache that match the predicate.
func (c *LRU[K, V]) RemoveMatching(predicate func(key K, value V) bool) {
	for k, e := range c.items {
		if predicate(k, e.Value.(entry[K, V]).value) {
			c.removeElement(e)
		}
	}
}
