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
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/singleflight"
)

type LoaderFunc[K comparable, V any] func(K) (V, error)

type LoadingLRUCacheWithTTL[K comparable, V any] struct {
	lru    *LRUCacheWithTTL[K, V]
	loader LoaderFunc[K, V]
	closer func() error
}

func NewLoadingLRUCacheWithTTL[K comparable, V any](reg prometheus.Registerer, maxEntries int, ttl time.Duration, loader LoaderFunc[K, V]) *LoadingLRUCacheWithTTL[K, V] {
	load := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "cache_load_total",
		Help: "Total number of successful cache loads.",
	}, []string{"result"})
	loadTotalTime := promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
		Name:                        "cache_load_duration_seconds",
		Help:                        "Total time spent loading cache.",
		NativeHistogramBucketFactor: 1.1,
	})
	return &LoadingLRUCacheWithTTL[K, V]{
		lru: NewLRUCacheWithTTL[K, V](reg, maxEntries, ttl),
		loader: func(k K) (V, error) {
			start := time.Now()
			v, err := loader(k)
			loadTotalTime.Observe(time.Since(start).Seconds())
			if err != nil {
				load.WithLabelValues("error").Inc()
			} else {
				load.WithLabelValues("success").Inc()
			}
			return v, err
		},
		closer: func() error {
			var err error

			if ok := reg.Unregister(load); !ok {
				err = errors.Join(err, fmt.Errorf("unregistering load counter: %w", err))
			}
			if ok := reg.Unregister(loadTotalTime); !ok {
				err = errors.Join(err, fmt.Errorf("unregistering load total time histogram: %w", err))
			}
			if err != nil {
				return fmt.Errorf("cleaning cache stats counter: %w", err)
			}
			return nil
		},
	}
}

func (c *LoadingLRUCacheWithTTL[K, V]) getOrLoad(key K) (V, error) {
	v, ok := c.lru.Get(key)
	if ok {
		return v, nil
	}

	v, err := c.loader(key)
	if err != nil {
		return v, err
	}

	c.lru.Add(key, v)
	return v, nil
}

func (c *LoadingLRUCacheWithTTL[K, V]) Get(key K) (V, error) {
	return c.getOrLoad(key)
}

func (c *LoadingLRUCacheWithTTL[K, V]) Close() error {
	var err error
	err = errors.Join(err, c.closer())
	err = errors.Join(err, c.lru.Close())
	return err
}

type LoadingOnceCache[K comparable, V any] struct {
	*LoadingLRUCacheWithTTL[K, V]

	sfg *singleflight.Group
}

// NewLoadingOnceCache creates a LoadingCache that allows only one loading operation at a time.
//
// The returned LoadingCache will call the loader function to load entries
// on cache misses. However, it will use a singleflight.Group to ensure only
// one concurrent call to the loader is made for a given key. This can be used
// to prevent redundant loading of data on cache misses when multiple concurrent
// requests are made for the same key.
func NewLoadingOnceCache[K comparable, V any](reg prometheus.Registerer, maxEntries int, ttl time.Duration, loader LoaderFunc[K, V]) *LoadingOnceCache[K, V] {
	c := &LoadingOnceCache[K, V]{
		NewLoadingLRUCacheWithTTL(reg, maxEntries, ttl, loader),
		&singleflight.Group{},
	}
	return c
}

func (c *LoadingOnceCache[K, V]) Get(key K) (V, error) {
	// singleflight.Group memoizes the return value of the first call and returns it.
	// The 3rd return value is true if multiple calls happens simultaneously,
	// and the caller received the value from the first call.
	val, err, _ := c.sfg.Do(fmt.Sprintf("%v", key), func() (interface{}, error) {
		return c.getOrLoad(key)
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return val.(V), nil //nolint:forcetypeassert
}
