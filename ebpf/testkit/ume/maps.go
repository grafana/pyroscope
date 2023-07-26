package ume

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/cilium/ebpf"
)

type Array struct {
	keySize    uint32
	valueSize  uint32
	maxEntries uint32
	data       []byte
}

func (a *Array) UpdateElem(k uintptr, v uintptr, flags uintptr) uintptr {
	//TODO implement me
	panic("implement me")
}

func (a *Array) PerfEventOutput(data uintptr, size uintptr, flags uintptr) uintptr {
	panic("wrong map")
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
	Data map[K]*V
}

func (h HashMap[K, V]) UpdateElem(k uintptr, v uintptr, flags uintptr) uintptr {
	//TODO implement me
	pk := (*K)(unsafe.Pointer(k))
	pv := (*V)(unsafe.Pointer(v))

	kk := *pk
	vv := *pv
	h.Data[kk] = &vv
	return 0
}

func (h HashMap[K, V]) PerfEventOutput(data uintptr, size uintptr, flags uintptr) uintptr {
	panic("wrong map")
}

func NewHashMap[K comparable, V any]() *HashMap[K, V] {
	return &HashMap[K, V]{
		Data: make(map[K]*V),
	}
}

func (h *HashMap[K, V]) Update(k, v any, flags ebpf.MapUpdateFlags) error {
	//TODO implement me
	kk, ok := k.(K)
	if !ok {
		panic(fmt.Sprintf("key type mismatch %v", k))
	}
	vv, ok := v.(V)
	if !ok {
		panic(fmt.Sprintf("value type mismatch %v", v))
	}

	if flags == ebpf.UpdateAny {
		h.Data[kk] = &vv
		return nil
	}
	_, ok = h.Data[kk]
	if flags == ebpf.UpdateNoExist {
		if ok {
			return fmt.Errorf("already exist %v ", k)
		}
		h.Data[kk] = &vv
		return nil
	}
	if flags == ebpf.UpdateExist {
		if !ok {
			return fmt.Errorf("does not exist %v ", k)
		}
		h.Data[kk] = &vv
	}
	panic(fmt.Sprintf("unknown flag %d", flags))
	return nil
}

type Entry[K comparable, V any] struct {
	K K
	V *V
}

func (h *HashMap[K, V]) Lookup(pkey uintptr) uintptr {
	k := *(*K)(unsafe.Pointer(pkey))
	v := h.Data[k]
	return uintptr(unsafe.Pointer(v))
}

func (h *HashMap[K, V]) Entries() []Entry[K, V] {
	res := make([]Entry[K, V], 0, len(h.Data))
	for k, v := range h.Data {
		res = append(res, Entry[K, V]{k, v})
	}
	return res
}

type PerfEventMap struct {
	ch chan []byte
}

func (p PerfEventMap) UpdateElem(k uintptr, v uintptr, flags uintptr) uintptr {
	//TODO implement me
	panic("implement me")
}

func NewPerfEventMap(sz int) *PerfEventMap {
	return &PerfEventMap{
		ch: make(chan []byte, sz),
	}
}

func (p *PerfEventMap) Lookup(pkey uintptr) uintptr {
	panic("wrong map")
}

func (p *PerfEventMap) PerfEventOutput(data uintptr, size uintptr, flags uintptr) uintptr {
	buf := make([]byte, size)
	memcpy_(uintptr(unsafe.Pointer(&buf[0])), data, size)
	select {
	case p.ch <- buf:
	default:
		fmt.Println("Channel full. Discarding perf event data")
	}
	return 0
}

func (p *PerfEventMap) Events() <-chan []byte {
	return p.ch
}
