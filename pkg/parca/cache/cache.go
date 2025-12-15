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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/parca/cache/lru"
)

type LRUCache[K comparable, V any] struct {
	lru *lru.LRU[K, V]
	mtx *sync.RWMutex
}

func NewLRUCache[K comparable, V any](reg prometheus.Registerer, maxEntries int) *LRUCache[K, V] {
	return &LRUCache[K, V]{
		lru: lru.New[K, V](reg, lru.WithMaxSize[K, V](maxEntries)),
		mtx: &sync.RWMutex{},
	}
}

func (c *LRUCache[K, V]) Add(key K, value V) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Add(key, value)
}

func (c *LRUCache[K, V]) Get(key K) (V, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return c.lru.Get(key)
}

// Peek returns the value associated with key without updating the "recently used"-ness of that key.
func (c *LRUCache[K, V]) Peek(key K) (V, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return c.lru.Peek(key)
}

func (c *LRUCache[K, V]) Remove(key K) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Remove(key)
}

func (c *LRUCache[K, V]) Purge() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.lru.Purge()
}

func (c *LRUCache[K, V]) Close() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.lru.Purge()
	return c.lru.Close()
}
