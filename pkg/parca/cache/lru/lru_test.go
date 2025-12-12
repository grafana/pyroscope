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

package lru

import (
	"container/list"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestLRU(t *testing.T) {
	l := New[int, int](prometheus.NewRegistry(), WithMaxSize[int, int](128))

	for i := 0; i < 256; i++ {
		l.Add(i, i)
	}
	require.Equal(t, 128, len(l.items))
	require.Equal(t, 128.0, testutil.ToFloat64(l.metrics.evictions))

	for i, k := range keys(l.items) {
		if v, ok := l.Get(k); !ok || v != k {
			t.Fatalf("bad key: %v (value: %v, ok: %t), i: %d", k, v, ok, i)
		}
	}

	for i := 0; i < 128; i++ {
		v, ok := l.Get(i)
		require.Zero(t, v)
		require.False(t, ok)
	}
	for i := 128; i < 256; i++ {
		v, ok := l.Get(i)
		require.NotZero(t, v)
		require.True(t, ok)
	}

	for i := 128; i < 192; i++ {
		l.Remove(i)
		if _, ok := l.Get(i); ok {
			t.Fatalf("should be deleted")
		}
	}

	for i := 192; i < 256; i++ {
		if v, ok := l.Get(i); !ok || v != i {
			t.Fatalf("bad key: %v (value: %v, ok: %t)", i, v, ok)
		}
	}
}

func keys[K comparable](m map[K]*list.Element) []K {
	ks := make([]K, len(m))
	i := 0
	for k := range m {
		ks[i] = k
		i++
	}
	return ks
}

func TestLRU_Add(t *testing.T) {
	l := New[int, int](prometheus.NewRegistry(), WithMaxSize[int, int](1))

	l.Add(1, 1)
	require.Equal(t, 0.0, testutil.ToFloat64(l.metrics.evictions))

	l.Add(2, 2)
	require.Equal(t, 1.0, testutil.ToFloat64(l.metrics.evictions))
}

// test that Peek doesn't update recent-ness.
func TestLRUPeek(t *testing.T) {
	l := New[int, int](prometheus.NewRegistry(), WithMaxSize[int, int](2))

	l.Add(1, 1)
	l.Add(2, 2)
	if v, ok := l.Peek(1); !ok || v != 1 {
		t.Errorf("1 should be set to 1: %v, %v", v, ok)
	}

	l.Add(3, 3)
	require.Equal(t, keyOrder(l), []int{3, 2})
}

func keyOrder[K comparable, V any](l *LRU[K, V]) []K {
	f := l.evictList.Front()
	if f == nil {
		return nil
	}
	var keys []K
	for e := f; e != nil; e = e.Next() {
		keys = append(keys, e.Value.(entry[K, V]).key) //nolint:forcetypeassert
	}
	return keys
}

func TestLRU_Remove(t *testing.T) {
	l := New[int, int](prometheus.NewRegistry(), WithMaxSize[int, int](2))

	l.Add(1, 1)
	l.Add(2, 2)
	l.Remove(1)
	if _, ok := l.Peek(1); ok {
		t.Errorf("1 should be removed")
	}
}
