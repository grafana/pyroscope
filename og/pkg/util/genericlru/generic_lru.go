package genericlru

import (
	"github.com/hashicorp/golang-lru/simplelru"
)

type GenericLRU[K any, V any] struct {
	lru *simplelru.LRU
}

type EvictCallback[K any, V any] func(k K, v *V)

func NewGenericLRU[K any, V any](sz int, evict EvictCallback[K, V]) (*GenericLRU[K, V], error) {
	lru, err := simplelru.NewLRU(sz, func(key interface{}, value interface{}) {
		evict(key.(K), value.(*V))
	})
	if err != nil {
		return nil, err
	}
	return &GenericLRU[K, V]{lru}, nil
}

func (l *GenericLRU[K, V]) Get(k K) (*V, bool) {
	v, ok := l.lru.Get(k)
	if ok {
		return v.(*V), ok
	}
	return nil, ok
}

func (l *GenericLRU[K, V]) Add(k K, v *V) (evicted bool) {
	return l.lru.Add(k, v)
}

func (l *GenericLRU[K, V]) Remove(k K) (present bool) {
	return l.lru.Remove(k)
}

func (l *GenericLRU[K, V]) Keys() (keys []K) {
	for _, key := range l.lru.Keys() {
		keys = append(keys, key.(K))
	}
	return keys
}

func (l *GenericLRU[K, V]) Len() int {
	return l.lru.Len()
}
