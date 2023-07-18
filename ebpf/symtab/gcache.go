package symtab

import (
	"fmt"
	lru "github.com/hashicorp/golang-lru/v2"
)

type Resource interface {
	Refresh()
	Cleanup()
}

type GCache[K comparable, V Resource] struct {
	options GCacheOptions

	roundCache map[K]*entry[V]
	lruCache   *lru.Cache[K, *entry[V]]

	round int
}
type entry[V Resource] struct {
	v     V
	round int
}

type GCacheOptions struct {
	Size       int
	KeepRounds int
}

func NewGCache[K comparable, V Resource](options GCacheOptions) (*GCache[K, V], error) {
	c, err := lru.NewWithEvict[K, *entry[V]](options.Size, func(key K, value *entry[V]) {
		value.v.Cleanup() // in theory this is not required, but add just in case
	})
	if err != nil {
		return nil, fmt.Errorf("lru create %w", err)
	}
	return &GCache[K, V]{
		options:    options,
		roundCache: make(map[K]*entry[V]),
		lruCache:   c,
	}, nil
}

func (g *GCache[K, V]) NextRound() {
	g.round++
}

func (g *GCache[K, V]) Get(k K) V {
	var zeroKey K
	var zeroVal V
	if k == zeroKey {
		return zeroVal
	}
	e, ok := g.lruCache.Get(k)
	if ok && e != nil {
		if e.round != g.round {
			e.round = g.round
			e.v.Refresh()
		}
		return e.v
	}
	e, ok = g.roundCache[k]
	if ok && e != nil {
		if e.round != g.round {
			e.round = g.round
			e.v.Refresh()
		}
		return e.v
	}
	return zeroVal
}

func (g *GCache[K, V]) Cache(k K, v V) {
	var zeroKey K
	if k == zeroKey {
		return
	}
	e := &entry[V]{v: v, round: g.round}
	e.v.Refresh()
	g.lruCache.Add(k, e)
	g.roundCache[k] = e
}

func (g *GCache[K, V]) Update(options GCacheOptions) {
	g.lruCache.Resize(options.Size)
	g.options = options
}

func (g *GCache[K, V]) Cleanup() {
	keys := g.lruCache.Keys()
	for _, pid := range keys {
		tab, ok := g.lruCache.Peek(pid)
		if !ok || tab == nil {
			continue
		}
		tab.v.Cleanup()
	}

	prev := g.roundCache
	next := make(map[K]*entry[V])
	for k, e := range prev {
		e.v.Cleanup()
		if e.round >= g.round-g.options.KeepRounds {
			next[k] = e
		}
	}
	g.roundCache = next

	//level.Debug(sc.logger).Log("msg", "symbolCache cleanup", "was", len(prev), "now", len(sc.roundCache))
}

func (g *GCache[K, V]) LRUSize() int {
	return g.lruCache.Len()
}

func (g *GCache[K, V]) Each(f func(k K, v V)) {
	g.EachLRU(f)
	g.EachRound(f)
}
func (g *GCache[K, V]) EachLRU(f func(k K, v V)) {
	keys := g.lruCache.Keys()
	for _, k := range keys {
		e, ok := g.lruCache.Peek(k)
		if !ok || e == nil {
			continue
		}
		f(k, e.v)
	}
}

func (g *GCache[K, V]) RoundSize() int {
	return len(g.roundCache)
}

func (g *GCache[K, V]) EachRound(f func(k K, v V)) {
	keys := g.lruCache.Keys()
	for _, k := range keys {
		e, ok := g.lruCache.Peek(k)
		if !ok || e == nil {
			continue
		}
		f(k, e.v)
	}
}

func (g *GCache[K, V]) Remove(k K) {
	g.lruCache.Remove(k)
	delete(g.roundCache, k)
}

type GCacheDebugInfo[T any] struct {
	LRUSize   int `river:"lru_size,attr,optional"`
	RoundSize int `river:"round_size,attr,optional"`
	LRU       []T `river:"lru_dump,block,optional"`
	Round     []T `river:"round_dump,block,optional"`
}

func DebugInfo[K comparable, V Resource, D any](g *GCache[K, V], ff func(K, V) D) GCacheDebugInfo[D] {
	res := GCacheDebugInfo[D]{
		LRUSize:   g.LRUSize(),
		RoundSize: g.RoundSize(),
		LRU:       make([]D, 0, g.LRUSize()),
		Round:     make([]D, 0, g.RoundSize()),
	}
	g.EachLRU(func(k K, v V) {
		res.LRU = append(res.LRU, ff(k, v))
	})
	g.EachRound(func(k K, v V) {
		res.Round = append(res.Round, ff(k, v))
	})
	return res
}
