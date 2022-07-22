//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"debug/elf"
	"unsafe"
) // import "fmt"

// import "encoding/hex"

// import "github.com/iovisor/gobpf/pkg/ksym"

/*
#cgo CFLAGS: -I/usr/include/bcc/compat
#cgo LDFLAGS: -lbcc
#include <bcc/bcc_common.h>
#include <bcc/libbpf.h>
#include <bcc/bcc_syms.h>
*/
import "C"

var globalCache *symbolCache

func init() {
	globalCache = newSymbolCache()
}

type bccSymbol struct {
	name         *C.char
	demangleName *C.char
	module       *C.char
	offset       C.ulonglong
}

type symbolCache struct {
	cachePerPid map[uint32]unsafe.Pointer
}

func newSymbolCache() *symbolCache {

	return &symbolCache{
		cachePerPid: make(map[uint32]unsafe.Pointer),
	}
}

func (sc *symbolCache) cache(pid uint32) unsafe.Pointer {
	if cache, ok := sc.cachePerPid[pid]; ok {
		return cache
	}
	pidC := C.int(pid)
	if pid == 0 {
		pidC = C.int(-1)
	}
	symbolOpt := C.struct_bcc_symbol_option{use_symbol_type: C.uint(1 << elf.STT_FUNC)}
	symbolOptC := (*C.struct_bcc_symbol_option)(unsafe.Pointer(&symbolOpt))
	cache := C.bcc_symcache_new(pidC, symbolOptC)
	sc.cachePerPid[pid] = cache
	return sc.cachePerPid[pid]
}

func (sc *symbolCache) bccResolve(pid uint32, addr uint64) (string, uint64, string) {
	symbol := C.struct_bcc_symbol{}
	var symbolC = (*C.struct_bcc_symbol)(unsafe.Pointer(&symbol))

	cache := sc.cache(pid)
	var res C.int
	if pid == 0 {
		res = C.bcc_symcache_resolve_no_demangle(cache, C.ulong(addr), symbolC)
	} else {
		res = C.bcc_symcache_resolve(cache, C.ulong(addr), symbolC)
	}

	// if res < 0 {
	// 	return "", fmt.Errorf("unable to locate symbol %x %d, %q", addr, res, symbol)
	// }

	if res < 0 {
		if symbol.offset > 0 {
			return "", uint64(symbol.offset), C.GoString(symbol.module)
		}
		return "", addr, ""
	}
	if pid == 0 {
		return C.GoString(symbol.name), uint64(symbol.offset), C.GoString(symbol.module)
	} else {
		defer C.bcc_symbol_free_demangle_name(symbolC)
		return C.GoString(symbol.demangle_name), uint64(symbol.offset), C.GoString(symbol.module)
	}
}

func (sc *symbolCache) sym(pid uint32, addr uint64) string {
	name, _, _ := sc.bccResolve(pid, addr)
	if name == "" {
		name = "[unknown]"
	}
	return name
}
