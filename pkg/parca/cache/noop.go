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

type noopCache[K comparable, V any] struct{}

func NewNoopCache[K comparable, V any]() *noopCache[K, V] {
	return &noopCache[K, V]{}
}

func (c *noopCache[K, V]) Add(key K, value V) {
}

func (c *noopCache[K, V]) Get(key K) (V, bool) {
	var zero V
	return zero, false
}

func (c *noopCache[K, V]) Peek(key K) (V, bool) {
	var zero V
	return zero, false
}

func (c *noopCache[K, V]) Remove(key K) {
}

func (c *noopCache[K, V]) Purge() {
}

func (c *noopCache[K, V]) Close() error {
	return nil
}
