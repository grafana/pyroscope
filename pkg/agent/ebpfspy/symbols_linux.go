//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"debug/elf"
	"fmt"
	"github.com/hashicorp/golang-lru/simplelru"
	"sync"
	"unsafe"
)

/*
#include "bcc_syms/bcc_syms.h"
*/
import "C"

const symbolUnknown = "[unknown]"

type symbolCache struct {
	pid2Cache *simplelru.LRU //todo wrap using generics
	mutex     sync.Mutex
}

type symbolCacheEntry struct {
	cache unsafe.Pointer
}

func newSymbolCache(pidCacheSize int) *symbolCache {
	pid2Cache, _ := simplelru.NewLRU(pidCacheSize, func(pid interface{}, entry interface{}) {
		fmt.Printf("pid cache evicted %v %v\n", pid, entry)
		e := entry.(*symbolCacheEntry)
		C.bcc_free_symcache(e.cache, C.int(pid.(uint32)))
	})
	return &symbolCache{
		pid2Cache: pid2Cache,
	}
}

func (sc *symbolCache) bccResolve(pid uint32, addr uint64) (string, uint64, string) {
	symbol := C.struct_bcc_symbol{}
	var symbolC = (*C.struct_bcc_symbol)(unsafe.Pointer(&symbol))

	e := sc.getOrCreateCacheEntry(pid)
	var res C.int
	if pid == 0 {
		res = C.bcc_symcache_resolve_no_demangle(e.cache, C.ulong(addr), symbolC)
	} else {
		res = C.bcc_symcache_resolve(e.cache, C.ulong(addr), symbolC)
		defer C.bcc_symbol_free_demangle_name(symbolC)
	}

	if res < 0 {
		if symbol.offset > 0 {
			return "", uint64(symbol.offset), C.GoString(symbol.module)
		}
		return "", addr, ""
	}
	if pid == 0 {
		return C.GoString(symbol.name), uint64(symbol.offset), C.GoString(symbol.module)
	} else {

		return C.GoString(symbol.demangle_name), uint64(symbol.offset), C.GoString(symbol.module)
	}
}

func (sc *symbolCache) sym(pid uint32, addr uint64) string {
	name, _, _ := sc.bccResolve(pid, addr)
	if name == "" {
		name = symbolUnknown
	}
	return name
}

func (sc *symbolCache) getOrCreateCacheEntry(pid uint32) *symbolCacheEntry {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	if cache, ok := sc.pid2Cache.Get(pid); ok {
		return cache.(*symbolCacheEntry)
	}
	pidC := C.int(pid)
	if pid == 0 {
		pidC = C.int(-1) // for KSyms
	}
	symbolOpt := C.struct_bcc_symbol_option{use_symbol_type: C.uint(1 << elf.STT_FUNC)}
	symbolOptC := (*C.struct_bcc_symbol_option)(unsafe.Pointer(&symbolOpt))
	cache := C.bcc_symcache_new(pidC, symbolOptC)
	e := &symbolCacheEntry{cache: cache}

	sc.pid2Cache.Add(pid, e)
	return e
}

func (sc *symbolCache) reset() {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	for _, pid := range sc.pid2Cache.Keys() {
		sc.pid2Cache.Remove(pid)
	}
}
func (sc *symbolCache) remove(pid uint32) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	fmt.Printf("removing pid %d\n", pid)
	sc.pid2Cache.Remove(pid)
}
