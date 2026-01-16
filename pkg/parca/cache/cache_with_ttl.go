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

package cache

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/parca/cache/lru"
)

type CacheWithTTLOptions struct {
	UpdateDeadlineOnGet bool
	RemoveExpiredOnAdd  bool
}

type valueWithDeadline[V any] struct {
	value    V
	deadline time.Time
}

type LRUCacheWithTTL[K comparable, V any] struct {
	lru *lru.LRU[K, valueWithDeadline[V]]
	mtx *sync.RWMutex

	ttl time.Duration

	updateDeadlineOnGet bool
	removeExpiredOnAdd  bool
	nextRemoveExpired   time.Time
}

func NewLRUCacheWithTTL[K comparable, V any](reg prometheus.Registerer, maxEntries int, ttl time.Duration, opts ...CacheWithTTLOptions) *LRUCacheWithTTL[K, V] {
	lruOpts := []lru.Option[K, valueWithDeadline[V]]{
		lru.WithMaxSize[K, valueWithDeadline[V]](maxEntries),
	}
	c := &LRUCacheWithTTL[K, V]{
		mtx: &sync.RWMutex{},
		ttl: ttl,
	}
	if len(opts) > 0 {
		c.updateDeadlineOnGet = opts[0].UpdateDeadlineOnGet
		c.removeExpiredOnAdd = opts[0].RemoveExpiredOnAdd
		if c.removeExpiredOnAdd {
			c.nextRemoveExpired = time.Now().Add(ttl)
			lruOpts = append(lruOpts, lru.WithOnAdded[K, valueWithDeadline[V]](func(key K, value valueWithDeadline[V]) {
				now := time.Now()
				if c.nextRemoveExpired.Before(now) {
					// Happens in "Add" inside a lock, so we don't need to lock here.
					c.lru.RemoveMatching(func(k K, v valueWithDeadline[V]) bool {
						return v.deadline.Before(now)
					})
					c.nextRemoveExpired = now.Add(ttl)
				}
			}))
		}
	}
	c.lru = lru.New[K, valueWithDeadline[V]](reg, lruOpts...)
	return c
}

func (c *LRUCacheWithTTL[K, V]) Add(key K, value V) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Add(key, valueWithDeadline[V]{
		value:    value,
		deadline: time.Now().Add(c.ttl),
	})
}

func (c *LRUCacheWithTTL[K, V]) Get(key K) (V, bool) {
	c.mtx.RLock()
	v, ok := c.lru.Get(key)
	c.mtx.RUnlock()
	if !ok {
		return v.value, false
	}
	if v.deadline.Before(time.Now()) {
		c.mtx.Lock()
		c.lru.Remove(key)
		c.mtx.Unlock()
		return v.value, false
	}
	if c.updateDeadlineOnGet {
		c.mtx.Lock()
		c.lru.Add(key, valueWithDeadline[V]{
			value:    v.value,
			deadline: time.Now().Add(c.ttl),
		})
		c.mtx.Unlock()
	}
	return v.value, true
}

func (c *LRUCacheWithTTL[K, V]) Peek(key K) (V, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	v, ok := c.lru.Peek(key)
	return v.value, ok
}

func (c *LRUCacheWithTTL[K, V]) Remove(key K) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Remove(key)
}

func (c *LRUCacheWithTTL[K, V]) Purge() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Purge()
}

func (c *LRUCacheWithTTL[K, V]) Close() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	return c.lru.Close()
}

type LRUCacheWithEvictionTTL[K comparable, V any] struct {
	lru *lru.LRU[K, valueWithDeadline[V]]
	mtx *sync.RWMutex

	ttl time.Duration
}

func NewLRUCacheWithEvictionTTL[K comparable, V any](reg prometheus.Registerer, maxEntries int, ttl time.Duration, onEvictedCallback func(k K, v V)) *LRUCacheWithEvictionTTL[K, V] {
	opts := []lru.Option[K, valueWithDeadline[V]]{
		lru.WithMaxSize[K, valueWithDeadline[V]](maxEntries),
		lru.WithOnEvict[K, valueWithDeadline[V]](func(k K, vd valueWithDeadline[V]) {
			onEvictedCallback(k, vd.value)
		}),
	}
	return &LRUCacheWithEvictionTTL[K, V]{
		lru: lru.New[K, valueWithDeadline[V]](reg, opts...),
		mtx: &sync.RWMutex{},
		ttl: ttl,
	}
}

func (c *LRUCacheWithEvictionTTL[K, V]) Add(key K, value V) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Add(key, valueWithDeadline[V]{
		value:    value,
		deadline: time.Now().Add(c.ttl),
	})
}

func (c *LRUCacheWithEvictionTTL[K, V]) Get(key K) (V, bool) {
	c.mtx.RLock()
	v, ok := c.lru.Get(key)
	c.mtx.RUnlock()
	if !ok {
		var zero V
		return zero, false
	}
	if v.deadline.Before(time.Now()) {
		c.mtx.Lock()
		c.lru.Remove(key)
		c.mtx.Unlock()
		var zero V
		return zero, false
	}
	return v.value, true
}

func (c *LRUCacheWithEvictionTTL[K, V]) Peek(key K) (V, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	v, ok := c.lru.Peek(key)
	return v.value, ok
}

func (c *LRUCacheWithEvictionTTL[K, V]) Remove(key K) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Remove(key)
}

func (c *LRUCacheWithEvictionTTL[K, V]) Purge() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Purge()
}

func (c *LRUCacheWithEvictionTTL[K, V]) Close() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	return c.lru.Close()
}
