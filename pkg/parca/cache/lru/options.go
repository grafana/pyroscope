// Copyright 2022-2025 The Parca Authors
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
//

package lru

type Option[K comparable, V any] func(lru *LRU[K, V])

func WithMaxSize[K comparable, V any](maxSize int) Option[K, V] {
	return func(lru *LRU[K, V]) {
		// Zero means no limit.
		lru.maxEntries = maxSize
	}
}

func WithOnEvict[K comparable, V any](onEvict func(key K, value V)) Option[K, V] {
	return func(lru *LRU[K, V]) {
		lru.onEvicted = onEvict
	}
}

func WithOnAdded[K comparable, V any](onAdded func(key K, value V)) Option[K, V] {
	return func(lru *LRU[K, V]) {
		lru.onAdded = onAdded
	}
}
