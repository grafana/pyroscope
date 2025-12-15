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
	"context"
	"errors"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/semaphore"

	"github.com/grafana/pyroscope/pkg/parca/cache/lru"
)

type LRUWithEviction[K comparable, V any] struct {
	lru *lru.LRU[K, V]
	mtx *sync.RWMutex

	onEvictedCallback func(k K, v V)
}

// NewLRUWithEviction returns a new CacheWithEviction with the given maxEntries.
func NewLRUWithEviction[K comparable, V any](reg prometheus.Registerer, maxEntries int, onEvictedCallback func(k K, v V)) (*LRUWithEviction[K, V], error) {
	if onEvictedCallback == nil {
		return nil, errors.New("onEvictedCallback must not be nil")
	}
	limiter := semaphore.NewWeighted(5)
	c := &LRUWithEviction[K, V]{
		mtx: &sync.RWMutex{},
		onEvictedCallback: func(k K, v V) {
			if err := limiter.Acquire(context.Background(), 1); err != nil {
				return
			}
			onEvictedCallback(k, v)
			limiter.Release(1)
		},
	}
	c.lru = lru.New[K, V](
		reg,
		lru.WithMaxSize[K, V](maxEntries),
		lru.WithOnEvict[K, V](c.onEvicted),
	)
	return c, nil
}

// onEvicted is called when an entry is evicted from the underlying LRU.
func (c *LRUWithEviction[K, V]) onEvicted(k K, v V) {
	go c.onEvictedCallback(k, v)
}

// Add adds a value to the cache.
func (c *LRUWithEviction[K, V]) Add(key K, value V) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Add(key, value)
}

// Get looks up a key's value from the cache.
func (c *LRUWithEviction[K, V]) Get(key K) (V, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return c.lru.Get(key)
}

// Peek returns the value associated with key without updating the "recently used"-ness of that key.
func (c *LRUWithEviction[K, V]) Peek(key K) (V, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return c.lru.Peek(key)
}

// Remove removes the provided key from the cache.
func (c *LRUWithEviction[K, V]) Remove(key K) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Remove(key)
}

// Purge is used to completely clear the cache.
func (c *LRUWithEviction[K, V]) Purge() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Purge()
}

// Close is used to close the underlying LRU by also purging it.
func (c *LRUWithEviction[K, V]) Close() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Close()
}
