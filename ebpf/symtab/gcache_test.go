package symtab

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type mockResource struct {
	name    string
	refresh int
	cleanup int
}

func (m *mockResource) Refresh() {
	m.refresh++
}

func (m *mockResource) Cleanup() {
	m.cleanup++
}

func (m *mockResource) DebugString() string {
	return "mock{}"
}

func TestGCache(t *testing.T) {
	cache, err := NewGCache[string, *mockResource](GCacheOptions{Size: 2, KeepRounds: 3})
	require.NoError(t, err)
	cache.NextRound()
	require.Equal(t, 1, cache.round)

	r1 := &mockResource{name: "r1"}
	r2 := &mockResource{name: "r2"}
	r3 := &mockResource{name: "r3"}

	res := cache.Get("k1")
	require.Nil(t, res)

	cache.Cache("k1", r1)
	res = cache.Get("k1")
	require.Equal(t, r1, res)
	require.Equal(t, 1, res.refresh)
	require.Equal(t, 0, res.cleanup)

	res = cache.Get("k1")
	require.Equal(t, 1, res.refresh)
	require.Equal(t, 0, res.cleanup)

	require.Equal(t, 1, len(cache.roundCache))
	require.Equal(t, 1, cache.lruCache.Len())

	cache.Cache("k2", r2)
	require.Equal(t, 1, r2.refresh)
	require.Equal(t, 0, r2.cleanup)
	require.Equal(t, 2, len(cache.roundCache))
	require.Equal(t, 2, cache.lruCache.Len())

	cache.Cache("k3", r3)
	require.Equal(t, 3, len(cache.roundCache))
	require.Equal(t, 2, cache.lruCache.Len())

	cache.Cleanup()
	require.NotEqual(t, 0, r1.cleanup)
	require.NotEqual(t, 0, r2.cleanup)
	require.NotEqual(t, 0, r3.cleanup)
	require.Equal(t, 2, cache.lruCache.Len())
	require.Equal(t, 3, len(cache.roundCache))

	// round 2
	cache.NextRound()
	require.Equal(t, 2, cache.round)

	r1.cleanup = 0
	r2.cleanup = 0
	r3.cleanup = 0
	r1.refresh = 0
	r2.refresh = 0
	r3.refresh = 0

	res = cache.Get("k1")
	require.Equal(t, r1, res)
	require.Equal(t, 1, r1.refresh)

	cache.Cleanup()
	require.NotEqual(t, 0, r1.cleanup)
	require.NotEqual(t, 0, r2.cleanup)
	require.NotEqual(t, 0, r3.cleanup)

	// round 3

	cache.NextRound()
	cache.Cache("k4", &mockResource{})
	cache.Cache("k5", &mockResource{})
	cache.Cleanup()

	// round 4
	cache.NextRound()
	cache.Cleanup()

	// round 5
	cache.NextRound()
	cache.Cleanup()

	require.Equal(t, 2, cache.lruCache.Len())
	require.Equal(t, 3, len(cache.roundCache))

	res = cache.Get("k1")
	require.Equal(t, res, r1)
	res = cache.Get("k2")
	require.Nil(t, res)
	res = cache.Get("k3")
	require.Nil(t, res)

	res = cache.Get("k4")
	require.NotNil(t, res)
	res = cache.Get("k5")
	require.NotNil(t, res)
}
