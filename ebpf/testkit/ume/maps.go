package ume

import (
	"fmt"
	"github.com/cilium/ebpf"
	"reflect"
	"unsafe"
)

type Array struct {
	keySize    uint32
	valueSize  uint32
	maxEntries uint32
	data       []byte
}

func (a *Array) Update(k, v any, flags ebpf.MapUpdateFlags) error {
	panic("implement me")
}

func NewArrayMap[T any](maxEntries uint32) *Array {
	var v T
	size := reflect.TypeOf(v).Size()
	res := Array{
		keySize:    4,
		valueSize:  uint32(size),
		maxEntries: maxEntries,
		data:       make([]byte, int(size)*int(maxEntries)),
	}
	return &res
}

// static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *) 1;
func (a *Array) Lookup(pkey uintptr) uintptr {
	key := *((*uint32)(unsafe.Pointer(pkey)))
	if key >= a.maxEntries || key < 0 {
		panic("not implemented")
	}
	o := key * a.valueSize
	return uintptr(unsafe.Pointer(&a.data[o]))
}

type HashMap[K comparable, V any] struct {
	data map[K]*V
}

func NewHashMap[K comparable, V any]() Map {
	return &HashMap[K, V]{
		data: make(map[K]*V),
	}
}

func (h HashMap[K, V]) Update(k, v any, flags ebpf.MapUpdateFlags) error {
	//TODO implement me
	kk, ok := k.(K)
	if !ok {
		panic(fmt.Sprintf("key type mismatch %v", k))
	}
	vv, ok := v.(V)
	if !ok {
		panic(fmt.Sprintf("value type mismatch %v", v))
	}
	//// UpdateAny creates a new element or update an existing one.
	//UpdateAny MapUpdateFlags = iota
	//// UpdateNoExist creates a new element.
	//UpdateNoExist MapUpdateFlags = 1 << (iota - 1)
	//// UpdateExist updates an existing element.
	//UpdateExist
	//// UpdateLock updates elements under bpf_spin_lock.
	//UpdateLock
	if flags == ebpf.UpdateAny {
		h.data[kk] = &vv
		return nil
	}
	_, ok = h.data[kk]
	if flags == ebpf.UpdateNoExist {
		if ok {
			return fmt.Errorf("already exist %v ", k)
		}
		h.data[kk] = &vv
		return nil
	}
	if flags == ebpf.UpdateExist {
		if !ok {
			return fmt.Errorf("does not exist %v ", k)
		}
		h.data[kk] = &vv
	}
	panic(fmt.Sprintf("unknown flag %d", flags))
	return nil
}

func (h HashMap[K, V]) Lookup(pkey uintptr) uintptr {
	//TODO implement me
	k := *(*K)(unsafe.Pointer(pkey))
	v := h.data[k]
	return uintptr(unsafe.Pointer(v))
}
